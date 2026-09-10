package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/setting"
	"github.com/ppxb/miyabi/internal/pan"
	"golang.org/x/sync/singleflight"
)

const panCredentialsSetting = "pan.credentials"

type panClient interface {
	Close()
	Account(context.Context, string) (pan.Account, error)
	BeginLogin(context.Context) (*pan.Login, error)
	LoginStatus(context.Context, *pan.Login) (pan.LoginState, error)
	ExchangeToken(context.Context, *pan.Login) (pan.Tokens, error)
	RefreshToken(context.Context, string) (pan.Tokens, error)
	List(context.Context, string, string, int, int) (pan.FilePage, error)
	Info(context.Context, string, string) (pan.FileInfo, error)
	ReadMetadata(context.Context, string, string, int64) ([]byte, error)
	UploadMetadata(context.Context, string, string, string, []byte) error
	AddOffline(context.Context, string, string, string) (string, error)
	RemoveOffline(context.Context, string, string) error
	OfflineTasks(context.Context, string, int) (pan.OfflinePage, error)
	PlayURL(context.Context, string, string) ([]pan.PlaySource, error)
	OpenMedia(context.Context, string, string, http.Header) (*http.Response, error)
}

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
	work  singleflight.Group
}

type PanService struct {
	database *ent.Client
	client   panClient

	// mu protects memory only. commit serializes persistence with publication.
	mu                   sync.Mutex
	commit               contextLock
	tokens               pan.Tokens
	session              *panLoginSession
	directory            panLibraryDirectory
	authorizationVersion uint64 // Login or mount changes invalidate source-bound work.
	credentialVersion    uint64 // Login or logout invalidates the previous account's work.
	tokenVersion         uint64 // Rotation groups requests that rejected the same token.
	refresh              singleflight.Group
	work                 sync.WaitGroup
	closed               bool
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
	service.mu.Lock()
	service.closed = true
	service.mu.Unlock()
	service.work.Wait()
	service.client.Close()
}

// Register before Close can begin waiting. Shared credential work may outlive
// its HTTP callers, but must finish persisting before the database closes.
func (service *PanService) startWork() bool {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.closed {
		return false
	}
	service.work.Add(1)
	return true
}

func (service *PanService) account(ctx context.Context, state panSnapshot) (pan.Account, error) {
	account, err := withPanToken(ctx, service, state, func(token string) (pan.Account, error) {
		return service.client.Account(ctx, token)
	})
	if err != nil {
		return pan.Account{}, fmt.Errorf("get 115 account: %w", err)
	}
	if _, err := service.credentials(state); err != nil {
		return pan.Account{}, err
	}
	return account, nil
}

func (service *PanService) Account(ctx context.Context) (PanAccountStatus, error) {
	state := service.snapshot()
	if state.tokens.AccessToken == "" {
		return PanAccountStatus{}, nil
	}
	account, err := service.account(ctx, state)
	if err != nil {
		return PanAccountStatus{}, err
	}
	if err := service.commit.Lock(ctx); err != nil {
		return PanAccountStatus{}, err
	}
	defer service.commit.Unlock()
	if _, err := service.credentials(state); err != nil {
		return PanAccountStatus{}, err
	}
	if err := service.discardOtherAccountDirectory(ctx, account.ID); err != nil {
		return PanAccountStatus{}, err
	}
	status := PanAccountStatus{Connected: true, Account: &account}
	directory := service.snapshot().directory
	if directory.AccountID == account.ID {
		value := directory.PanLibraryDirectory
		status.Directory = &value
	}
	return status, nil
}

func (service *PanService) BeginLogin(ctx context.Context) (PanLoginSession, error) {
	if err := service.commit.Lock(ctx); err != nil {
		return PanLoginSession{}, err
	}
	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		service.commit.Unlock()
		return PanLoginSession{}, context.Canceled
	}
	session := &panLoginSession{id: uuid.NewString(), state: pan.LoginWaiting}
	service.session = session
	service.mu.Unlock()
	service.commit.Unlock()

	login, err := service.client.BeginLogin(ctx)
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.session != session || service.closed {
		return PanLoginSession{}, fmt.Errorf("115 登录会话已变更，请重新扫码")
	}
	if err != nil {
		service.session = nil
		return PanLoginSession{}, fmt.Errorf("start 115 login: %w", err)
	}
	session.login = login
	return PanLoginSession{
		ID: session.id, QRCode: "data:image/png;base64," + base64.StdEncoding.EncodeToString(login.QRCode),
	}, nil
}

func (service *PanService) LoginStatus(ctx context.Context, id string) (PanLoginStatus, error) {
	if err := ctx.Err(); err != nil {
		return PanLoginStatus{}, err
	}
	service.mu.Lock()
	session := service.session
	if session == nil || session.id != id || session.login == nil && session.err == nil && session.state == pan.LoginWaiting {
		service.mu.Unlock()
		return PanLoginStatus{State: pan.LoginExpired}, nil
	}
	service.mu.Unlock()
	result := session.work.DoChan("status", func() (any, error) {
		if !service.startWork() {
			return PanLoginStatus{}, context.Canceled
		}
		defer service.work.Done()
		return service.pollLogin(ctx, session)
	})
	select {
	case <-ctx.Done():
		return PanLoginStatus{}, ctx.Err()
	case completed := <-result:
		if completed.Err != nil {
			return PanLoginStatus{}, completed.Err
		}
		return completed.Val.(PanLoginStatus), nil
	}
}

func (service *PanService) pollLogin(ctx context.Context, session *panLoginSession) (PanLoginStatus, error) {
	service.mu.Lock()
	if service.session != session {
		service.mu.Unlock()
		return PanLoginStatus{State: pan.LoginExpired}, nil
	}
	if session.err != nil || session.state != pan.LoginWaiting && session.state != pan.LoginScanned {
		state, err := session.state, session.err
		service.mu.Unlock()
		return PanLoginStatus{State: state}, err
	}
	login := session.login
	service.mu.Unlock()

	pollContext, stopPoll := context.WithTimeout(context.WithoutCancel(ctx), 35*time.Second)
	state, err := service.client.LoginStatus(pollContext, login)
	stopPoll()
	if err != nil {
		return PanLoginStatus{}, fmt.Errorf("poll 115 login: %w", err)
	}
	if state == pan.LoginAuthorized {
		err = service.completeLogin(ctx, session, login)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.session != session {
		return PanLoginStatus{State: pan.LoginExpired}, nil
	}
	if err != nil {
		session.err = err
		return PanLoginStatus{}, err
	}
	session.state = state
	if state != pan.LoginWaiting && state != pan.LoginScanned {
		session.login = nil
	}
	return PanLoginStatus{State: state}, nil
}

func (service *PanService) completeLogin(ctx context.Context, session *panLoginSession, login *pan.Login) error {
	// Exchanging a device code consumes it. A departing browser only stops
	// waiting; the bounded exchange and persistence still finish.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer cancel()
	service.mu.Lock()
	current := service.session == session
	service.mu.Unlock()
	if !current {
		return nil
	}
	tokens, err := service.client.ExchangeToken(ctx, login)
	if err != nil {
		return fmt.Errorf("complete 115 login: %w", err)
	}
	if err := service.commit.Lock(ctx); err != nil {
		return err
	}
	service.mu.Lock()
	current = service.session == session
	service.mu.Unlock()
	if !current {
		service.commit.Unlock()
		return nil
	}
	if err := saveSetting(ctx, service.database, panCredentialsSetting, tokens); err != nil {
		service.commit.Unlock()
		return err
	}
	service.mu.Lock()
	service.tokens = tokens
	service.tokenVersion++
	service.credentialVersion++
	service.authorizationVersion++
	service.mu.Unlock()
	state := service.snapshot()
	service.commit.Unlock()

	account, err := service.account(ctx, state)
	if err != nil {
		return fmt.Errorf("verify 115 login account: %w", err)
	}
	if err := service.commit.Lock(ctx); err != nil {
		return err
	}
	defer service.commit.Unlock()
	if _, err := service.credentials(state); err != nil {
		return err
	}
	return service.discardOtherAccountDirectory(ctx, account.ID)
}

func (service *PanService) Disconnect(ctx context.Context) (PanAccountStatus, error) {
	if err := service.commit.Lock(ctx); err != nil {
		return PanAccountStatus{}, err
	}
	defer service.commit.Unlock()
	if _, err := service.database.Setting.Delete().Where(setting.Key(panCredentialsSetting)).Exec(ctx); err != nil {
		return PanAccountStatus{}, fmt.Errorf("remove 115 credentials: %w", err)
	}
	service.mu.Lock()
	service.tokens = pan.Tokens{}
	service.credentialVersion++
	service.authorizationVersion++
	service.tokenVersion++
	service.session = nil
	service.mu.Unlock()
	return PanAccountStatus{}, nil
}
