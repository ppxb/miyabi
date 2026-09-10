package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/pan"
)

type panStub struct {
	panClient
	account        func(context.Context, string) (pan.Account, error)
	beginLogin     func(context.Context) (*pan.Login, error)
	loginStatus    func(context.Context, *pan.Login) (pan.LoginState, error)
	exchangeToken  func(context.Context, *pan.Login) (pan.Tokens, error)
	refreshToken   func(context.Context, string) (pan.Tokens, error)
	list           func(context.Context, string, string, int, int) (pan.FilePage, error)
	info           func(context.Context, string, string) (pan.FileInfo, error)
	uploadMetadata func(context.Context, string, string, string, []byte) error
	addOffline     func(context.Context, string, string, string) (string, error)
	removeOffline  func(context.Context, string, string) error
	offlineTasks   func(context.Context, string, int) (pan.OfflinePage, error)
}

func (client *panStub) Close() {
	if client.panClient != nil {
		client.panClient.Close()
	}
}

func (client *panStub) Account(ctx context.Context, token string) (pan.Account, error) {
	return client.account(ctx, token)
}

func (client *panStub) BeginLogin(ctx context.Context) (*pan.Login, error) {
	return client.beginLogin(ctx)
}

func (client *panStub) LoginStatus(ctx context.Context, login *pan.Login) (pan.LoginState, error) {
	return client.loginStatus(ctx, login)
}

func (client *panStub) ExchangeToken(ctx context.Context, login *pan.Login) (pan.Tokens, error) {
	return client.exchangeToken(ctx, login)
}

func (client *panStub) RefreshToken(ctx context.Context, token string) (pan.Tokens, error) {
	return client.refreshToken(ctx, token)
}

func (client *panStub) List(ctx context.Context, token, directory string, offset, limit int) (pan.FilePage, error) {
	return client.list(ctx, token, directory, offset, limit)
}

func (client *panStub) Info(ctx context.Context, token, id string) (pan.FileInfo, error) {
	return client.info(ctx, token, id)
}

func (client *panStub) UploadMetadata(ctx context.Context, token, directory, name string, body []byte) error {
	return client.uploadMetadata(ctx, token, directory, name, body)
}

func (client *panStub) AddOffline(ctx context.Context, token, uri, directory string) (string, error) {
	return client.addOffline(ctx, token, uri, directory)
}

func (client *panStub) RemoveOffline(ctx context.Context, token, hash string) error {
	return client.removeOffline(ctx, token, hash)
}

func (client *panStub) OfflineTasks(ctx context.Context, token string, page int) (pan.OfflinePage, error) {
	return client.offlineTasks(ctx, token, page)
}

func panTestTokens(prefix string) pan.Tokens {
	return pan.Tokens{AccessToken: prefix + "-access", RefreshToken: prefix + "-refresh", ExpiresAt: time.Now().Add(time.Hour)}
}

func panConcurrencyFixture(t *testing.T) (*LibraryService, *panStub) {
	t.Helper()
	library, _, payload := libraryFixture(t)
	client := &panStub{
		account: func(context.Context, string) (pan.Account, error) {
			return pan.Account{ID: payload.Source.AccountID}, nil
		},
		beginLogin:  func(context.Context) (*pan.Login, error) { return &pan.Login{QRCode: []byte("fixture")}, nil },
		loginStatus: func(context.Context, *pan.Login) (pan.LoginState, error) { return pan.LoginAuthorized, nil },
	}
	library.drive = &PanService{
		database: library.database, client: client, tokens: panTestTokens("original"),
		directory: panLibraryDirectory{AccountID: payload.Source.AccountID, PanLibraryDirectory: payload.Source.Directory},
	}
	if err := saveSetting(t.Context(), library.database, panCredentialsSetting, library.drive.tokens); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(library.drive.Close)
	return library, client
}

func panTestGate(t *testing.T) (<-chan struct{}, func()) {
	t.Helper()
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	t.Cleanup(release)
	return gate, release
}

func awaitPan[T any](t *testing.T, ready <-chan T) T {
	t.Helper()
	select {
	case value := <-ready:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for concurrent 115 operation")
		var zero T
		return zero
	}
}

func awaitPanCondition(t *testing.T, ready func() bool) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for !ready() {
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("concurrent 115 operation did not reach the expected state")
		}
	}
}

func assertPanTokens(t *testing.T, drive *PanService, want pan.Tokens) {
	t.Helper()
	saved, exists, err := loadSetting[pan.Tokens](t.Context(), drive.database, panCredentialsSetting)
	if err != nil || exists != (want.AccessToken != "") {
		t.Fatalf("persisted credentials: exists=%t err=%v", exists, err)
	}
	for _, got := range []pan.Tokens{saved, drive.snapshot().tokens} {
		if got.AccessToken != want.AccessToken || got.RefreshToken != want.RefreshToken || !got.ExpiresAt.Equal(want.ExpiresAt) {
			t.Fatalf("credentials = %+v, want %+v", got, want)
		}
	}
}

func TestPanRequestsDoNotBlockPlaybackAuthorization(t *testing.T) {
	play, source := playFixture(t)
	drive := play.library.drive
	started, release := make(chan struct{}), make(chan struct{})
	drive.client = &panStub{panClient: drive.client, list: func(context.Context, string, string, int, int) (pan.FilePage, error) {
		close(started)
		<-release
		return pan.FilePage{}, nil
	}}
	playback, err := play.createSession(source, 0, []pan.PlaySource{{URL: "https://cdn.example/video", Height: 1080}})
	if err != nil {
		t.Fatal(err)
	}
	listed := make(chan error, 1)
	go func() {
		_, err := drive.Files(t.Context(), "10", 1)
		listed <- err
	}()
	defer func() { close(release); <-listed }()
	<-started
	checked := make(chan error, 1)
	go func() {
		_, _, err := play.resource(playback.ID, 0)
		checked <- err
	}()
	select {
	case err := <-checked:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("playback authorization waited for an unrelated 115 request")
	}
}
