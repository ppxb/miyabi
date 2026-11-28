package drive

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/pan"
	"golang.org/x/sync/singleflight"
)

const guangyaLoginLifetime = 2 * time.Minute

// guangyaLogin owns the device authorization session used by QR login. The
// session lives only in memory.
type guangyaLogin struct {
	drive   *Drive
	session *guangyaLoginSession
}

type guangyaLoginSession struct {
	id         string
	deviceCode string
	qrCodeURL  string
	expiresAt  time.Time
	state      pan.LoginState
	accountID  string
	tokens     *pan.Tokens
	work       singleflight.Group
}

// GuangyaLoginSession is returned after a QR login starts.
type GuangyaLoginSession struct {
	ID        string `json:"id"`
	QRCodeURL string `json:"qr_code_url"`
}

// BeginGuangyaLogin starts the device authorization flow and returns the URL
// that the web client renders as a QR code.
func (d *Drive) BeginGuangyaLogin(ctx context.Context) (GuangyaLoginSession, error) {
	if d.guangya == nil {
		return GuangyaLoginSession{}, domain.E(domain.KindInvalid, "光鸭登录未配置", nil)
	}
	requestContext, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()
	login, err := d.guangya.BeginDeviceLogin(requestContext)
	if err != nil {
		return GuangyaLoginSession{}, err
	}
	lifetime := time.Duration(login.ExpiresIn) * time.Second
	if lifetime <= 0 {
		lifetime = guangyaLoginLifetime
	}
	session := &guangyaLoginSession{
		id:         uuid.NewString(),
		deviceCode: login.DeviceCode,
		qrCodeURL:  login.VerificationURIComplete,
		expiresAt:  time.Now().Add(lifetime),
	}
	if err := d.commit.Lock(ctx); err != nil {
		return GuangyaLoginSession{}, err
	}
	defer d.commit.Unlock()
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return GuangyaLoginSession{}, context.Canceled
	}
	d.guangyaLogin.session = session
	return GuangyaLoginSession{ID: session.id, QRCodeURL: session.qrCodeURL}, nil
}

// GuangyaLoginStatus polls the device authorization once and stores the
// account after the user confirms on the phone. Terminal states stay on the
// session so repeated polls return the same answer.
func (d *Drive) GuangyaLoginStatus(ctx context.Context, id string) (LoginStatus, error) {
	if d.guangya == nil {
		return LoginStatus{}, domain.E(domain.KindInvalid, "光鸭登录未配置", nil)
	}
	d.mu.Lock()
	session := d.guangyaLogin.session
	if session == nil {
		d.mu.Unlock()
		return LoginStatus{State: pan.LoginExpired}, nil
	}
	if session.id != id {
		d.mu.Unlock()
		return LoginStatus{State: pan.LoginCanceled}, nil
	}
	if session.state != "" {
		status := LoginStatus{State: session.state, AccountID: session.accountID}
		d.mu.Unlock()
		return status, nil
	}
	if time.Now().After(session.expiresAt) {
		session.state = pan.LoginExpired
		d.mu.Unlock()
		return LoginStatus{State: pan.LoginExpired}, nil
	}
	d.mu.Unlock()

	result := session.work.DoChan("status", func() (any, error) {
		done, ok := d.StartWork()
		if !ok {
			return LoginStatus{}, context.Canceled
		}
		defer done()
		return d.pollGuangyaLogin(ctx, session)
	})
	select {
	case <-ctx.Done():
		return LoginStatus{}, ctx.Err()
	case completed := <-result:
		if completed.Err != nil {
			return LoginStatus{}, completed.Err
		}
		return completed.Val.(LoginStatus), nil
	}
}

func (d *Drive) pollGuangyaLogin(ctx context.Context, session *guangyaLoginSession) (LoginStatus, error) {
	d.mu.Lock()
	if d.guangyaLogin.session != session {
		d.mu.Unlock()
		return LoginStatus{State: pan.LoginCanceled}, nil
	}
	deviceCode := session.deviceCode
	cachedTokens := session.tokens
	d.mu.Unlock()
	if cachedTokens != nil {
		return d.completeGuangyaLogin(ctx, session, *cachedTokens)
	}

	// The token exchange and account write must outlive the browser request:
	// closing the dialog while the phone confirms would otherwise lose the
	// already-issued tokens.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), upstreamTimeout)
	defer cancel()
	tokens, state, err := d.guangya.PollDeviceLogin(ctx, deviceCode)
	if err != nil {
		return LoginStatus{}, err
	}
	switch state {
	case pan.LoginAuthorized:
		// Keep the issued token on the session: if the account lookup or write
		// fails, the next poll retries that step instead of exchanging again.
		d.mu.Lock()
		if d.guangyaLogin.session == session {
			issued := tokens
			session.tokens = &issued
		}
		d.mu.Unlock()
		return d.completeGuangyaLogin(ctx, session, tokens)
	case pan.LoginExpired, pan.LoginCanceled:
		d.finishGuangyaLogin(session, state, "")
		return LoginStatus{State: state}, nil
	default:
		return LoginStatus{State: state}, nil
	}
}

func (d *Drive) completeGuangyaLogin(ctx context.Context, session *guangyaLoginSession, tokens pan.Tokens) (LoginStatus, error) {
	accountContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), upstreamTimeout)
	defer cancel()
	account, err := d.guangya.Account(accountContext, tokens.AccessToken)
	if err != nil {
		// A login endpoint must not surface as an expired existing account:
		// the frontend would refresh unrelated pan/library state.
		if errors.Is(err, pan.ErrUnauthorized) {
			return LoginStatus{}, domain.E(domain.KindUpstream, "光鸭账号信息读取失败，请重新扫码", nil)
		}
		return LoginStatus{}, err
	}
	return d.acceptGuangyaLogin(accountContext, session, account, tokens)
}

func (d *Drive) acceptGuangyaLogin(ctx context.Context, session *guangyaLoginSession, account pan.Account, tokens pan.Tokens) (LoginStatus, error) {
	if err := d.commit.Lock(ctx); err != nil {
		return LoginStatus{}, err
	}
	defer d.commit.Unlock()
	if d.closedState() {
		return LoginStatus{}, context.Canceled
	}
	d.mu.Lock()
	current := d.guangyaLogin.session == session
	d.mu.Unlock()
	if !current {
		return LoginStatus{State: pan.LoginCanceled}, nil
	}
	if err := d.saveAccountTokens(ctx, account.ID, tokens, ProviderGuangya); err != nil {
		return LoginStatus{}, err
	}
	d.bumpAccountCredential(account.ID)
	d.bumpAccountAuthorization(account.ID)
	d.finishGuangyaLogin(session, pan.LoginAuthorized, account.ID)
	return LoginStatus{State: pan.LoginAuthorized, AccountID: account.ID}, nil
}

func (d *Drive) finishGuangyaLogin(session *guangyaLoginSession, state pan.LoginState, accountID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.guangyaLogin.session != session {
		return
	}
	session.state = state
	session.accountID = accountID
}
