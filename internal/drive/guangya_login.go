package drive

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/domain"
)

const guangyaLoginLifetime = 10 * time.Minute

// normalizeGuangyaPhone accepts common Chinese input forms and returns E.164.
// Sign-in is stricter than sending the SMS, so the number must be normalized
// before the whole login flow.
func normalizeGuangyaPhone(phone string) string {
	value := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(strings.TrimSpace(phone))
	switch {
	case value == "":
		return ""
	case strings.HasPrefix(value, "+"):
		return value
	case strings.HasPrefix(value, "00"):
		return "+" + value[2:]
	case len(value) == 11 && value[0] == '1':
		return "+86" + value
	case len(value) == 13 && strings.HasPrefix(value, "86"):
		return "+" + value
	default:
		return value
	}
}

// guangyaLogin owns the two-step SMS login session: one phone number, one
// verification id, then one code. The session lives only in memory.
type guangyaLogin struct {
	drive   *Drive
	session *guangyaLoginSession
}

type guangyaLoginSession struct {
	id             string
	phone          string
	captchaToken   string
	verificationID string
	startedAt      time.Time
}

// GuangyaLoginSession is returned after the SMS has been sent.
type GuangyaLoginSession struct {
	ID string `json:"id"`
}

// BeginGuangyaLogin sends the SMS and returns a session id for the verify step.
func (d *Drive) BeginGuangyaLogin(ctx context.Context, phone string) (GuangyaLoginSession, error) {
	if d.guangya == nil {
		return GuangyaLoginSession{}, domain.E(domain.KindInvalid, "光鸭登录未配置", nil)
	}
	phone = normalizeGuangyaPhone(phone)
	if phone == "" {
		return GuangyaLoginSession{}, domain.E(domain.KindInvalid, "请输入手机号", nil)
	}
	requestContext, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()
	captchaToken, err := d.guangya.SMSInit(requestContext, phone)
	if err != nil {
		return GuangyaLoginSession{}, err
	}
	verificationID, err := d.guangya.SMSSend(requestContext, phone, captchaToken)
	if err != nil {
		return GuangyaLoginSession{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return GuangyaLoginSession{}, context.Canceled
	}
	session := &guangyaLoginSession{
		id:             uuid.NewString(),
		phone:          phone,
		captchaToken:   captchaToken,
		verificationID: verificationID,
		startedAt:      time.Now(),
	}
	d.guangyaLogin.session = session
	return GuangyaLoginSession{ID: session.id}, nil
}

// VerifyGuangyaLogin consumes the SMS code, stores the account and returns it.
func (d *Drive) VerifyGuangyaLogin(ctx context.Context, id, code string) (AccountStatus, error) {
	if d.guangya == nil {
		return AccountStatus{}, domain.E(domain.KindInvalid, "光鸭登录未配置", nil)
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return AccountStatus{}, domain.E(domain.KindInvalid, "请输入验证码", nil)
	}
	d.mu.Lock()
	session := d.guangyaLogin.session
	if session == nil || session.id != id || time.Since(session.startedAt) > guangyaLoginLifetime {
		d.mu.Unlock()
		return AccountStatus{}, domain.E(domain.KindInvalid, "登录会话已失效，请重新获取验证码", nil)
	}
	phone, captchaToken, verificationID := session.phone, session.captchaToken, session.verificationID
	d.mu.Unlock()

	requestContext, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()
	verificationToken, err := d.guangya.SMSVerify(requestContext, verificationID, code)
	if err != nil {
		return AccountStatus{}, err
	}
	tokens, err := d.guangya.SignIn(requestContext, phone, code, verificationToken, captchaToken)
	if err != nil {
		return AccountStatus{}, err
	}
	account, err := d.guangya.Account(requestContext, tokens.AccessToken)
	if err != nil {
		return AccountStatus{}, err
	}

	if err := d.commit.Lock(ctx); err != nil {
		return AccountStatus{}, err
	}
	defer d.commit.Unlock()
	if d.closedState() {
		return AccountStatus{}, context.Canceled
	}
	d.mu.Lock()
	if d.guangyaLogin.session == session {
		d.guangyaLogin.session = nil
	}
	d.mu.Unlock()
	if err := d.saveAccountTokens(ctx, account.ID, tokens, ProviderGuangya); err != nil {
		return AccountStatus{}, err
	}
	d.bumpAccountCredential(account.ID)
	d.bumpAccountAuthorization(account.ID)
	return AccountStatus{
		AccountID: account.ID,
		Provider:  ProviderGuangya,
		Connected: true,
		Account:   &account,
	}, nil
}
