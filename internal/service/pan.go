package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/setting"
	"github.com/ppxb/miyabi/internal/pan"
)

const panCredentialsSetting = "pan.credentials"

type PanAccountStatus struct {
	Connected bool                 `json:"connected"`
	Account   *pan.Account         `json:"account,omitempty"`
	Directory *PanLibraryDirectory `json:"directory,omitempty"`
}

type PanLoginSession struct {
	ID     string `json:"id"`
	QRCode string `json:"qr_code"`
}

type PanLoginStatus struct {
	State pan.LoginState `json:"state"`
}

type panLoginSession struct {
	id    string
	login *pan.Login
	state pan.LoginState
	err   error
}

type PanService struct {
	database *ent.Client
	client   *pan.Client

	// A single account owns both token rotation and the active login session.
	mu        sync.Mutex
	tokens    pan.Tokens
	session   *panLoginSession
	directory panLibraryDirectory
}

func NewPanService(ctx context.Context, database *ent.Client, options pan.Options) (*PanService, error) {
	tokens, _, err := loadSetting[pan.Tokens](ctx, database, panCredentialsSetting)
	if err != nil {
		return nil, err
	}
	directory, _, err := loadSetting[panLibraryDirectory](ctx, database, panDirectorySetting)
	if err != nil {
		return nil, err
	}
	return &PanService{database: database, client: pan.New(options), tokens: tokens, directory: directory}, nil
}

func (service *PanService) Close() {
	service.client.Close()
}

func (service *PanService) Account(ctx context.Context) (PanAccountStatus, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.tokens.AccessToken == "" {
		return PanAccountStatus{}, nil
	}

	account, err := withPanToken(ctx, service, func(token string) (pan.Account, error) {
		return service.client.Account(ctx, token)
	})
	if err != nil {
		return PanAccountStatus{}, fmt.Errorf("get 115 account: %w", err)
	}
	status := PanAccountStatus{Connected: true, Account: &account}
	if service.directory.AccountID == account.ID {
		directory := service.directory.PanLibraryDirectory
		status.Directory = &directory
	}
	return status, nil
}

func (service *PanService) BeginLogin(ctx context.Context) (PanLoginSession, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return PanLoginSession{}, err
	}
	service.session = nil
	login, err := service.client.BeginLogin(ctx)
	if err != nil {
		return PanLoginSession{}, fmt.Errorf("start 115 login: %w", err)
	}
	session := &panLoginSession{id: uuid.NewString(), login: login, state: pan.LoginWaiting}
	service.session = session
	return PanLoginSession{
		ID:     session.id,
		QRCode: "data:image/png;base64," + base64.StdEncoding.EncodeToString(login.QRCode),
	}, nil
}

func (service *PanService) LoginStatus(ctx context.Context, id string) (PanLoginStatus, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	session := service.session
	if session == nil || session.id != id {
		return PanLoginStatus{State: pan.LoginExpired}, nil
	}
	if session.err != nil {
		return PanLoginStatus{}, session.err
	}
	if session.state != pan.LoginWaiting && session.state != pan.LoginScanned {
		return PanLoginStatus{State: session.state}, nil
	}
	state, err := service.client.LoginStatus(ctx, session.login)
	if err != nil {
		return PanLoginStatus{}, fmt.Errorf("poll 115 login: %w", err)
	}
	if state == pan.LoginAuthorized {
		// Exchanging a device code consumes it. Finish saving the credentials
		// even if the browser closes the dialog while the request is running.
		tokenContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
		defer cancel()
		tokens, err := service.client.ExchangeToken(tokenContext, session.login)
		if err != nil {
			session.err = fmt.Errorf("complete 115 login: %w", err)
			return PanLoginStatus{}, session.err
		}
		if err := service.saveTokens(tokenContext, tokens); err != nil {
			session.err = err
			return PanLoginStatus{}, err
		}
	}
	session.state = state
	if state != pan.LoginWaiting && state != pan.LoginScanned {
		session.login = nil
	}
	return PanLoginStatus{State: state}, nil
}

func (service *PanService) Disconnect(ctx context.Context) (PanAccountStatus, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if _, err := service.database.Setting.Delete().Where(setting.Key(panCredentialsSetting)).Exec(ctx); err != nil {
		return PanAccountStatus{}, fmt.Errorf("remove 115 credentials: %w", err)
	}
	service.tokens = pan.Tokens{}
	service.session = nil
	return PanAccountStatus{}, nil
}

// Callers hold mu, so concurrent requests reuse the newly persisted token pair.
func (service *PanService) refreshTokens(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Refresh tokens rotate too; a canceled caller must not discard the new pair.
	tokenContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer cancel()
	tokens, err := service.client.RefreshToken(tokenContext, service.tokens.RefreshToken)
	if err != nil {
		return fmt.Errorf("refresh 115 credentials: %w", err)
	}
	return service.saveTokens(tokenContext, tokens)
}

func (service *PanService) saveTokens(ctx context.Context, tokens pan.Tokens) error {
	if err := saveSetting(ctx, service.database, panCredentialsSetting, tokens); err != nil {
		return err
	}
	service.tokens = tokens
	return nil
}

// Callers hold mu. Only rejected authorization is replayed after refreshing tokens.
func withPanToken[T any](ctx context.Context, service *PanService, request func(string) (T, error)) (T, error) {
	var zero T
	if service.tokens.AccessToken == "" {
		return zero, pan.ErrUnauthorized
	}
	refreshed := time.Until(service.tokens.ExpiresAt) <= 30*time.Second
	if refreshed {
		if err := service.refreshTokens(ctx); err != nil {
			return zero, err
		}
	}
	value, err := request(service.tokens.AccessToken)
	if errors.Is(err, pan.ErrUnauthorized) && !refreshed {
		if err := service.refreshTokens(ctx); err != nil {
			return zero, err
		}
		return request(service.tokens.AccessToken)
	}
	return value, err
}
