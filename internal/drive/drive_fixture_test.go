package drive

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/pan"
)

var testSource = domain.LibrarySource{
	AccountID: "100",
	Directory: domain.LibraryDirectory{ID: "10", Name: "Movies", Path: "/Movies"},
}

// stubClient is a 115 client whose calls can be replaced per test. Unset
// calls behave like a healthy account so fixtures need no boilerplate.
type stubClient struct {
	account        func(context.Context, string) (pan.Account, error)
	beginLogin     func(context.Context) (*pan.Login, error)
	loginStatus    func(context.Context, *pan.Login) (pan.LoginState, error)
	exchangeToken  func(context.Context, *pan.Login) (pan.Tokens, error)
	refreshToken   func(context.Context, string) (pan.Tokens, error)
	list           func(context.Context, string, string, int, int) (pan.FilePage, error)
	info           func(context.Context, string, string) (pan.FileInfo, error)
	readMetadata   func(context.Context, string, string, int64) ([]byte, error)
	uploadMetadata func(context.Context, string, string, string, []byte) error
	addOffline     func(context.Context, string, string, string) (string, error)
	removeOffline  func(context.Context, string, string) error
	offlineTasks   func(context.Context, string, int) (pan.OfflinePage, error)
	playURL        func(context.Context, string, string) ([]pan.PlaySource, error)
	downloadURL    func(context.Context, string, string) (string, error)
}

func (c *stubClient) Close() {}

func (c *stubClient) Account(ctx context.Context, token string) (pan.Account, error) {
	if c.account != nil {
		return c.account(ctx, token)
	}
	return pan.Account{ID: testSource.AccountID}, nil
}

func (c *stubClient) BeginLogin(ctx context.Context) (*pan.Login, error) {
	if c.beginLogin != nil {
		return c.beginLogin(ctx)
	}
	return &pan.Login{QRCode: []byte("fixture")}, nil
}

func (c *stubClient) LoginStatus(ctx context.Context, login *pan.Login) (pan.LoginState, error) {
	if c.loginStatus != nil {
		return c.loginStatus(ctx, login)
	}
	return pan.LoginAuthorized, nil
}

func (c *stubClient) ExchangeToken(ctx context.Context, login *pan.Login) (pan.Tokens, error) {
	if c.exchangeToken != nil {
		return c.exchangeToken(ctx, login)
	}
	return testTokens("login"), nil
}

func (c *stubClient) RefreshToken(ctx context.Context, token string) (pan.Tokens, error) {
	if c.refreshToken != nil {
		return c.refreshToken(ctx, token)
	}
	return testTokens("refreshed"), nil
}

func (c *stubClient) List(ctx context.Context, token, directory string, offset, limit int) (pan.FilePage, error) {
	if c.list != nil {
		return c.list(ctx, token, directory, offset, limit)
	}
	return directoryPage(domain.LibraryDirectory{ID: directory, Name: "Movies" + directory, Path: "/Movies" + directory}), nil
}

func (c *stubClient) Info(ctx context.Context, token, id string) (pan.FileInfo, error) {
	if c.info != nil {
		return c.info(ctx, token, id)
	}
	return pan.FileInfo{File: pan.File{ID: id, Name: "video.mp4", PickCode: "pick-" + id}}, nil
}

func (c *stubClient) ReadMetadata(ctx context.Context, token, pickCode string, limit int64) ([]byte, error) {
	if c.readMetadata != nil {
		return c.readMetadata(ctx, token, pickCode, limit)
	}
	return []byte("<movie/>"), nil
}

func (c *stubClient) UploadMetadata(ctx context.Context, token, directory, name string, body []byte) error {
	if c.uploadMetadata != nil {
		return c.uploadMetadata(ctx, token, directory, name, body)
	}
	return nil
}

func (c *stubClient) AddOffline(ctx context.Context, token, uri, directory string) (string, error) {
	if c.addOffline != nil {
		return c.addOffline(ctx, token, uri, directory)
	}
	return "", nil
}

func (c *stubClient) RemoveOffline(ctx context.Context, token, hash string) error {
	if c.removeOffline != nil {
		return c.removeOffline(ctx, token, hash)
	}
	return nil
}

func (c *stubClient) OfflineTasks(ctx context.Context, token string, page int) (pan.OfflinePage, error) {
	if c.offlineTasks != nil {
		return c.offlineTasks(ctx, token, page)
	}
	return pan.OfflinePage{PageCount: 1}, nil
}

func (c *stubClient) PlayURL(ctx context.Context, token, pickCode string) ([]pan.PlaySource, error) {
	if c.playURL != nil {
		return c.playURL(ctx, token, pickCode)
	}
	return []pan.PlaySource{{URL: "https://cdn.example/video.m3u8", Height: 1080}}, nil
}

func (c *stubClient) DownloadURL(ctx context.Context, token, pickCode, userAgent string) (string, error) {
	if c.downloadURL != nil {
		return c.downloadURL(ctx, token, pickCode)
	}
	return "https://cdn.example/download/" + pickCode, nil
}

func (c *stubClient) OpenMedia(context.Context, string, string, http.Header) (*http.Response, error) {
	return nil, fmt.Errorf("media streaming is not stubbed")
}

// directoryPage is what 115 answers when a directory is listed: its ancestry
// chain. Mounting it yields exactly the given LibraryDirectory.
func directoryPage(directory domain.LibraryDirectory) pan.FilePage {
	page := pan.FilePage{Path: []pan.Directory{{ID: "0", Name: "Root"}}}
	segments := strings.Split(strings.Trim(directory.Path, "/"), "/")
	for i, name := range segments[:len(segments)-1] {
		page.Path = append(page.Path, pan.Directory{ID: fmt.Sprintf("ancestor-%d", i), Name: name})
	}
	name := directory.Name
	if name == "" {
		name = segments[len(segments)-1]
	}
	page.Path = append(page.Path, pan.Directory{ID: directory.ID, Name: name})
	return page
}

func testTokens(prefix string) pan.Tokens {
	return pan.Tokens{AccessToken: prefix + "-access", RefreshToken: prefix + "-refresh", ExpiresAt: time.Now().Add(time.Hour)}
}

func newTestDrive(t testing.TB, client *stubClient) *Drive {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	d, err := NewWithClient(t.Context(), store.Client, client)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d
}

// loginTestAccount completes a QR login through the real state machine; the
// stub's Account answer decides which account gets logged in.
func loginTestAccount(t testing.TB, d *Drive) {
	t.Helper()
	login, err := d.BeginLogin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	status, err := d.LoginStatus(t.Context(), login.ID)
	if err != nil || status.State != pan.LoginAuthorized {
		t.Fatalf("fixture login = %+v, %v", status, err)
	}
}

// selectTestDirectory mounts the directory as the library root, answering the
// mount listing so the stored directory equals the given value.
func selectTestDirectory(t testing.TB, d *Drive, client *stubClient, directory domain.LibraryDirectory) {
	t.Helper()
	previous := client.list
	client.list = func(ctx context.Context, token, id string, offset, limit int) (pan.FilePage, error) {
		if id == directory.ID {
			return directoryPage(directory), nil
		}
		if previous != nil {
			return previous(ctx, token, id, offset, limit)
		}
		return pan.FilePage{}, nil
	}
	defer func() { client.list = previous }()
	if _, err := d.SelectDirectory(t.Context(), directory.ID); err != nil {
		t.Fatal(err)
	}
}

func mountTestSource(t testing.TB, d *Drive, client *stubClient, source domain.LibrarySource) {
	t.Helper()
	client.account = func(context.Context, string) (pan.Account, error) { return pan.Account{ID: source.AccountID}, nil }
	loginTestAccount(t, d)
	if source.Directory.ID != "" {
		selectTestDirectory(t, d, client, source.Directory)
	}
}

func mountedTestDrive(t testing.TB) (*Drive, *stubClient) {
	t.Helper()
	client := &stubClient{}
	d := newTestDrive(t, client)
	mountTestSource(t, d, client, testSource)
	return d, client
}

func testGate(t testing.TB) (<-chan struct{}, func()) {
	t.Helper()
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	t.Cleanup(release)
	return gate, release
}

func await[T any](t testing.TB, ready <-chan T) T {
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

func awaitCondition(t testing.TB, ready func() bool) {
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

func assertTokens(t testing.TB, d *Drive, want pan.Tokens) {
	t.Helper()
	saved, exists, err := database.LoadSetting[pan.Tokens](t.Context(), d.database, credentialsSetting)
	if err != nil || exists != (want.AccessToken != "") {
		t.Fatalf("persisted credentials: exists=%t err=%v", exists, err)
	}
	for _, got := range []pan.Tokens{saved, d.snapshot().tokens} {
		if got.AccessToken != want.AccessToken || got.RefreshToken != want.RefreshToken || !got.ExpiresAt.Equal(want.ExpiresAt) {
			t.Fatalf("credentials = %+v, want %+v", got, want)
		}
	}
}

func expireTokens(d *Drive) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tokens.ExpiresAt = time.Now()
}

func savedDirectory(t testing.TB, d *Drive) (mountRecord, bool) {
	t.Helper()
	record, found, err := database.LoadSetting[mountRecord](t.Context(), d.database, directorySetting)
	if err != nil {
		t.Fatal(err)
	}
	return record, found
}
