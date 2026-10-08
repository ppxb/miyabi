package offline

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
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/offlinedownload"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/library"
	scrapePkg "github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

type panStub struct {
	drive.Client
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
	openMedia      func(context.Context, string, string, http.Header) (*http.Response, error)
}

func (client *panStub) Close() {
	if client.Client != nil {
		client.Client.Close()
	}
}

func (client *panStub) Account(ctx context.Context, token string) (pan.Account, error) {
	if client.account != nil {
		return client.account(ctx, token)
	}
	if client.Client != nil {
		return client.Client.Account(ctx, token)
	}
	return pan.Account{}, nil
}

func (client *panStub) BeginLogin(ctx context.Context) (*pan.Login, error) {
	if client.beginLogin != nil {
		return client.beginLogin(ctx)
	}
	if client.Client != nil {
		return client.Client.BeginLogin(ctx)
	}
	return &pan.Login{QRCode: []byte("fixture")}, nil
}

func (client *panStub) LoginStatus(ctx context.Context, login *pan.Login) (pan.LoginState, error) {
	if client.loginStatus != nil {
		return client.loginStatus(ctx, login)
	}
	if client.Client != nil {
		return client.Client.LoginStatus(ctx, login)
	}
	return pan.LoginAuthorized, nil
}

func (client *panStub) ExchangeToken(ctx context.Context, login *pan.Login) (pan.Tokens, error) {
	if client.exchangeToken != nil {
		return client.exchangeToken(ctx, login)
	}
	if client.Client != nil {
		return client.Client.ExchangeToken(ctx, login)
	}
	return panTestTokens("login"), nil
}

func (client *panStub) RefreshToken(ctx context.Context, token string) (pan.Tokens, error) {
	if client.refreshToken != nil {
		return client.refreshToken(ctx, token)
	}
	if client.Client != nil {
		return client.Client.RefreshToken(ctx, token)
	}
	return panTestTokens("refreshed"), nil
}

func (client *panStub) List(ctx context.Context, token, directory string, offset, limit int) (pan.FilePage, error) {
	if client.list != nil {
		return client.list(ctx, token, directory, offset, limit)
	}
	if client.Client != nil {
		return client.Client.List(ctx, token, directory, offset, limit)
	}
	return pan.FilePage{}, nil
}

func (client *panStub) Info(ctx context.Context, token, fileID string) (pan.FileInfo, error) {
	if client.info != nil {
		return client.info(ctx, token, fileID)
	}
	if client.Client != nil {
		return client.Client.Info(ctx, token, fileID)
	}
	return pan.FileInfo{}, nil
}

func (client *panStub) AddOffline(ctx context.Context, token, directoryID, magnet string) (string, error) {
	if client.addOffline != nil {
		return client.addOffline(ctx, token, directoryID, magnet)
	}
	if client.Client != nil {
		return client.Client.AddOffline(ctx, token, directoryID, magnet)
	}
	return "fixture-info-hash", nil
}

func (client *panStub) RemoveOffline(ctx context.Context, token, hash string) error {
	if client.removeOffline != nil {
		return client.removeOffline(ctx, token, hash)
	}
	if client.Client != nil {
		return client.Client.RemoveOffline(ctx, token, hash)
	}
	return nil
}

func (client *panStub) OfflineTasks(ctx context.Context, token string, page int) (pan.OfflinePage, error) {
	if client.offlineTasks != nil {
		return client.offlineTasks(ctx, token, page)
	}
	if client.Client != nil {
		return client.Client.OfflineTasks(ctx, token, page)
	}
	return pan.OfflinePage{}, nil
}

func panTestTokens(prefix string) pan.Tokens {
	return pan.Tokens{AccessToken: prefix + "-access", RefreshToken: prefix + "-refresh", ExpiresAt: time.Now().Add(time.Hour)}
}

var fixtureStubs sync.Map

func stubOf(t testing.TB, d *drive.Drive) *panStub {
	t.Helper()
	client, ok := fixtureStubs.Load(d)
	if !ok {
		t.Fatal("drive was not created by newMountedDrive")
	}
	return client.(*panStub)
}

func newMountedDrive(t testing.TB, database *ent.Client, client *panStub, source domain.LibrarySource) *drive.Drive {
	t.Helper()
	d, err := drive.NewWithClient(t.Context(), database, client)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	fixtureStubs.Store(d, client)
	mountSource(t, d, client, source)
	return d
}

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

func loginAccount(t testing.TB, d *drive.Drive, client *panStub, accountID string) {
	t.Helper()
	client.account = func(context.Context, string) (pan.Account, error) { return pan.Account{ID: accountID}, nil }
	login, err := d.BeginLogin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status, err := d.LoginStatus(t.Context(), login.ID); err != nil || status.State != pan.LoginAuthorized {
		t.Fatalf("fixture login = %+v, %v", status, err)
	}
}

func mountSource(t testing.TB, d *drive.Drive, client *panStub, source domain.LibrarySource) {
	t.Helper()
	status, err := d.Account(t.Context())
	if err != nil || !status.Connected || status.Account == nil || status.Account.ID != source.AccountID {
		loginAccount(t, d, client, source.AccountID)
	}
	if source.Directory.ID == "" {
		return
	}
	previous := client.list
	client.list = func(ctx context.Context, token, id string, offset, limit int) (pan.FilePage, error) {
		if id == source.Directory.ID {
			return directoryPage(source.Directory), nil
		}
		if previous != nil {
			return previous(ctx, token, id, offset, limit)
		}
		return pan.FilePage{}, nil
	}
	defer func() { client.list = previous }()
	if _, err := d.SelectDirectory(t.Context(), source.Directory.ID); err != nil {
		t.Fatal(err)
	}
}

func libraryBaseFixture(t testing.TB) (*Service, *drive.Drive, *ent.Client, domain.LibrarySource) {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	source := domain.LibrarySource{AccountID: "100", Directory: domain.LibraryDirectory{ID: "10", Name: "Movies", Path: "/Movies"}}
	driveSvc := newMountedDrive(t, store.Client, &panStub{}, source)
	taskSvc := tasks.NewService(store.Client, tasks.NewRegistry())
	images, err := mediaimage.NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lib := library.New(store.Client, driveSvc, taskSvc, images, library.Options{Pacing: func(context.Context) error { return nil }})
	if _, err := lib.EnqueueScan(t.Context(), source); err != nil {
		t.Fatal(err)
	}

	scrapeSvc := scrapePkg.New(store.Client, driveSvc, nil, images, taskSvc, scrapePkg.Dependencies{})
	t.Cleanup(scrapeSvc.Close)
	taskSvc.Registry().Register(tasks.NewHandler(tasks.KindScan, lib.Scan, lib.Finished))
	taskSvc.Registry().Register(tasks.NewHandler(tasks.KindScrape, scrapeSvc.Scrape, scrapeSvc.Finished))

	cat := &stubCatalogue{
		movies: map[string]domain.MovieDetail{
			"fixture-movie": {Movie: domain.Movie{ID: "fixture-movie", Code: "ABP-001"}},
		},
		magnets: map[string][]domain.Magnet{
			"fixture-movie": {{Hash: offlineHashA}, {Hash: offlineHashB}},
		},
	}
	service := New(store.Client, cat, driveSvc, taskSvc, lib, 2*time.Minute)
	return service, driveSvc, store.Client, source
}

func offlineFixture(t testing.TB) (*Service, *ent.OfflineDownload, ent.OfflineDownload, domain.LibrarySource) {
	t.Helper()
	service, _, db, source := libraryBaseFixture(t)
	input := ent.OfflineDownload{
		AccountID:   source.AccountID,
		DirectoryID: source.Directory.ID,
		Code:        "ABP-001",
		JavdbID:     "fixture-movie",
		Hash:        "fixture-hash",
		InfoHash:    "fixture-hash",
	}

	record, err := createDownload(db, input).SetStatus(offlinedownload.StatusRunning).
		Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return service, record, input, source
}

type stubCatalogue struct {
	movies  map[string]domain.MovieDetail
	magnets map[string][]domain.Magnet
}

func (s *stubCatalogue) CatalogueMagnets(ctx context.Context, movieID string) ([]domain.Magnet, error) {
	return s.magnets[movieID], nil
}

func (s *stubCatalogue) HasMagnet(ctx context.Context, movieID, hash string) (bool, error) {
	for _, m := range s.magnets[movieID] {
		if strings.EqualFold(m.Hash, hash) {
			return true, nil
		}
	}
	return false, nil
}

func (s *stubCatalogue) MovieCode(ctx context.Context, movieID string) (string, error) {
	if m, ok := s.movies[movieID]; ok {
		return m.Code, nil
	}
	return "", nil
}

const (
	offlineHashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	offlineHashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func offlineAddFixture(t *testing.T) (*Service, *panStub) {
	t.Helper()
	service, driveSvc, _, _ := libraryBaseFixture(t)
	return service, stubOf(t, driveSvc)
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

func createDownload(db *ent.Client, input ent.OfflineDownload) *ent.OfflineDownloadCreate {
	return db.OfflineDownload.Create().SetCode(input.Code).SetJavdbID(input.JavdbID).
		SetHash(input.Hash).SetInfoHash(input.InfoHash).SetAccountID(input.AccountID).SetDirectoryID(input.DirectoryID).
		SetFileID(input.FileID).SetFileIds(input.FileIds).SetScanTaskID(input.ScanTaskID).SetAwaitingLocation(input.AwaitingLocation)
}
