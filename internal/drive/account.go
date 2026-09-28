package drive

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/setting"
	"github.com/ppxb/miyabi/internal/pan"
	"golang.org/x/sync/singleflight"
)

const (
	loginLifetime   = 10 * time.Minute
	accountCacheTTL = 60 * time.Second
	// upstreamTimeout bounds credential exchanges that outlive the caller's request.
	upstreamTimeout = 45 * time.Second
)

var loginPollWindow = 30 * time.Second

// AccountStatus describes current 115 connection and mounted library directory.
type AccountStatus struct {
	Connected bool                     `json:"connected"`
	Account   *pan.Account             `json:"account,omitempty"`
	Directory *domain.LibraryDirectory `json:"directory,omitempty"`
}

// LoginSession represents an active QR code login attempt.
type LoginSession struct {
	ID     string `json:"id"`
	QRCode string `json:"qr_code"`
}

// LoginStatus represents the current state of a QR login session.
type LoginStatus struct {
	State pan.LoginState `json:"state"`
}

type loginSession struct {
	id        string
	login     *pan.Login
	state     pan.LoginState
	err       error
	startedAt time.Time
	work      singleflight.Group
}

func (d *Drive) verifyAccount(ctx context.Context, state snapshot) (pan.Account, error) {
	d.accountCacheMu.Lock()
	if d.cachedAccount.ID != "" && d.cachedCredentialVersion == state.credentialVersion && time.Since(d.cachedAccountTime) < accountCacheTTL {
		cached := d.cachedAccount
		d.accountCacheMu.Unlock()
		return cached, nil
	}
	d.accountCacheMu.Unlock()

	account, err := d.fetchAccount(ctx, state)
	if err != nil {
		return pan.Account{}, err
	}

	d.accountCacheMu.Lock()
	defer d.accountCacheMu.Unlock()
	if _, err := d.credentials(state); err != nil {
		return pan.Account{}, err
	}
	d.cachedAccount = account
	d.cachedAccountTime = time.Now()
	d.cachedCredentialVersion = state.credentialVersion
	return account, nil
}

func (d *Drive) invalidateAccountCache() {
	d.accountCacheMu.Lock()
	d.cachedAccount = pan.Account{}
	d.cachedAccountTime = time.Time{}
	d.cachedCredentialVersion = 0
	d.accountCacheMu.Unlock()
}

func (d *Drive) fetchAccount(ctx context.Context, state snapshot) (pan.Account, error) {
	account, err := withPanToken(ctx, d, state, func(token string) (pan.Account, error) {
		return d.client.Account(ctx, token)
	})
	if err != nil {
		return pan.Account{}, fmt.Errorf("get 115 account: %w", err)
	}
	if _, err := d.credentials(state); err != nil {
		return pan.Account{}, err
	}
	return account, nil
}

func (d *Drive) Account(ctx context.Context) (AccountStatus, error) {
	state := d.snapshot()
	if state.tokens.AccessToken == "" {
		return AccountStatus{}, nil
	}
	account, err := d.verifyAccount(ctx, state)
	if err != nil {
		return AccountStatus{}, err
	}
	if err := d.commit.Lock(ctx); err != nil {
		return AccountStatus{}, err
	}
	defer d.commit.Unlock()
	if _, err := d.credentials(state); err != nil {
		return AccountStatus{}, err
	}
	if err := d.discardOtherAccountDirectory(ctx, account.ID); err != nil {
		return AccountStatus{}, err
	}
	status := AccountStatus{Connected: true, Account: &account}
	directory := d.snapshot().directory
	if directory.AccountID == account.ID {
		value := directory.LibraryDirectory
		status.Directory = &value
	}
	return status, nil
}

func (d *Drive) BeginLogin(ctx context.Context) (LoginSession, error) {
	if err := d.commit.Lock(ctx); err != nil {
		return LoginSession{}, err
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		d.commit.Unlock()
		return LoginSession{}, context.Canceled
	}
	session := &loginSession{id: uuid.NewString(), state: pan.LoginWaiting, startedAt: time.Now()}
	d.session = session
	d.mu.Unlock()
	d.commit.Unlock()

	login, err := d.client.BeginLogin(ctx)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.session != session || d.closed {
		return LoginSession{}, domain.E(domain.KindConflict, "115 登录会话已变更，请重新扫码", nil)
	}
	if err != nil {
		d.session = nil
		return LoginSession{}, fmt.Errorf("start 115 login: %w", err)
	}
	session.login = login
	return LoginSession{
		ID: session.id, QRCode: "data:image/png;base64," + base64.StdEncoding.EncodeToString(login.QRCode),
	}, nil
}

func (d *Drive) LoginStatus(ctx context.Context, id string) (LoginStatus, error) {
	if err := ctx.Err(); err != nil {
		return LoginStatus{}, err
	}
	d.mu.Lock()
	session := d.session
	if session == nil || session.id != id || session.login == nil && session.err == nil && session.state == pan.LoginWaiting {
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
		return d.pollLogin(ctx, session)
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

func (d *Drive) pollLogin(ctx context.Context, session *loginSession) (LoginStatus, error) {
	d.mu.Lock()
	if d.session != session {
		d.mu.Unlock()
		return LoginStatus{State: pan.LoginExpired}, nil
	}
	if session.err != nil || session.state != pan.LoginWaiting && session.state != pan.LoginScanned {
		state, err := session.state, session.err
		d.mu.Unlock()
		return LoginStatus{State: state}, err
	}
	if time.Since(session.startedAt) > loginLifetime {
		session.state = pan.LoginExpired
		session.login = nil
		d.mu.Unlock()
		return LoginStatus{State: pan.LoginExpired}, nil
	}
	login := session.login
	d.mu.Unlock()

	pollContext, stopPoll := context.WithTimeout(context.WithoutCancel(ctx), loginPollWindow)
	state, err := d.client.LoginStatus(pollContext, login)
	elapsed := errors.Is(pollContext.Err(), context.DeadlineExceeded)
	stopPoll()
	if err != nil {
		if !elapsed {
			return LoginStatus{}, fmt.Errorf("poll 115 login: %w", err)
		}
		state, err = pan.LoginWaiting, nil
	}
	if state == pan.LoginAuthorized {
		err = d.completeLogin(ctx, session, login)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.session != session {
		return LoginStatus{State: pan.LoginExpired}, nil
	}
	if err != nil {
		session.err = err
		return LoginStatus{}, err
	}
	if state == pan.LoginWaiting && session.state == pan.LoginScanned {
		state = pan.LoginScanned
	}
	session.state = state
	if state != pan.LoginWaiting && state != pan.LoginScanned {
		session.login = nil
	}
	return LoginStatus{State: state}, nil
}

func (d *Drive) completeLogin(ctx context.Context, session *loginSession, login *pan.Login) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), upstreamTimeout)
	defer cancel()
	d.mu.Lock()
	current := d.session == session
	d.mu.Unlock()
	if !current {
		return nil
	}
	tokens, err := d.client.ExchangeToken(ctx, login)
	if err != nil {
		return fmt.Errorf("complete 115 login: %w", err)
	}
	if err := d.commit.Lock(ctx); err != nil {
		return err
	}
	d.mu.Lock()
	current = d.session == session
	d.mu.Unlock()
	if !current {
		d.commit.Unlock()
		return nil
	}
	if err := database.SaveSetting(ctx, d.database, credentialsSetting, tokens); err != nil {
		d.commit.Unlock()
		return err
	}
	d.mu.Lock()
	d.tokens = tokens
	d.tokenVersion++
	d.credentialVersion++
	d.authorizationVersion++
	d.mu.Unlock()
	d.invalidateAccountCache()
	state := d.snapshot()
	d.commit.Unlock()

	account, err := d.fetchAccount(ctx, state)
	if err != nil {
		return fmt.Errorf("verify 115 login account: %w", err)
	}
	if err := d.commit.Lock(ctx); err != nil {
		return err
	}
	defer d.commit.Unlock()
	if _, err := d.credentials(state); err != nil {
		return err
	}
	return d.discardOtherAccountDirectory(ctx, account.ID)
}

func (d *Drive) Disconnect(ctx context.Context) (AccountStatus, error) {
	if err := d.commit.Lock(ctx); err != nil {
		return AccountStatus{}, err
	}
	defer d.commit.Unlock()
	if _, err := d.database.Setting.Delete().Where(setting.Key(credentialsSetting)).Exec(ctx); err != nil {
		return AccountStatus{}, fmt.Errorf("remove 115 credentials: %w", err)
	}
	d.mu.Lock()
	d.tokens = pan.Tokens{}
	d.credentialVersion++
	d.authorizationVersion++
	d.tokenVersion++
	d.session = nil
	d.mu.Unlock()
	d.invalidateAccountCache()
	return AccountStatus{}, nil
}
