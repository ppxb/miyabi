package library

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
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/library/scan"
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

func panTestTokens(prefix string) pan.Tokens {
	return pan.Tokens{AccessToken: prefix + "-access", RefreshToken: prefix + "-refresh", ExpiresAt: time.Now().Add(time.Hour)}
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
	return panTestTokens("refresh"), nil
}

func (client *panStub) List(ctx context.Context, token, dirID string, offset, limit int) (pan.FilePage, error) {
	if client.list != nil {
		return client.list(ctx, token, dirID, offset, limit)
	}
	if client.Client != nil {
		return client.Client.List(ctx, token, dirID, offset, limit)
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
	return pan.FileInfo{File: pan.File{ID: fileID}}, nil
}

func (client *panStub) ReadMetadata(ctx context.Context, token, pickCode string, limit int64) ([]byte, error) {
	if client.readMetadata != nil {
		return client.readMetadata(ctx, token, pickCode, limit)
	}
	if client.Client != nil {
		return client.Client.ReadMetadata(ctx, token, pickCode, limit)
	}
	return nil, nil
}

func (client *panStub) UploadMetadata(ctx context.Context, token, dirID, name string, body []byte) error {
	if client.uploadMetadata != nil {
		return client.uploadMetadata(ctx, token, dirID, name, body)
	}
	if client.Client != nil {
		return client.Client.UploadMetadata(ctx, token, dirID, name, body)
	}
	return nil
}

func (client *panStub) AddOffline(ctx context.Context, token, dirID, magnet string) (string, error) {
	if client.addOffline != nil {
		return client.addOffline(ctx, token, dirID, magnet)
	}
	if client.Client != nil {
		return client.Client.AddOffline(ctx, token, dirID, magnet)
	}
	return "hash", nil
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

func (client *panStub) PlayURL(ctx context.Context, token, pickCode string) ([]pan.PlaySource, error) {
	if client.playURL != nil {
		return client.playURL(ctx, token, pickCode)
	}
	if client.Client != nil {
		return client.Client.PlayURL(ctx, token, pickCode)
	}
	return []pan.PlaySource{{URL: "https://example.com/video.mp4"}}, nil
}

func (client *panStub) OpenMedia(ctx context.Context, token, url string, header http.Header) (*http.Response, error) {
	if client.openMedia != nil {
		return client.openMedia(ctx, token, url, header)
	}
	if client.Client != nil {
		return client.Client.OpenMedia(ctx, token, url, header)
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}, nil
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

func libraryFixture(t testing.TB) (*Service, tasks.TaskInfo, scan.Payload) {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	source := domain.LibrarySource{AccountID: "100", Directory: domain.LibraryDirectory{ID: "10", Name: "Movies", Path: "/Movies"}}
	driveSvc := newMountedDrive(t, store.Client, &panStub{}, source)
	taskSvc := tasks.NewService(store.Client, tasks.NewRegistry())
	queued, err := taskSvc.EnqueueScan(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	images, err := mediaimage.NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lib := New(store.Client, driveSvc, taskSvc, images)
	scrape := scrapePkg.New(store.Client, driveSvc, nil, images, taskSvc)
	taskSvc.Registry().Register(tasks.NewHandler(tasks.KindScan, lib.Scan, lib.Finished))
	taskSvc.Registry().Register(tasks.NewHandler(tasks.KindScrape, scrape.Scrape, scrape.Finished))
	taskSvc.Registry().Register(tasks.NewHandler(tasks.KindCover, scrape.Cover, scrape.Finished))
	return lib, queued, scan.Payload{Source: source, Scan: domain.ScanProgress{Stage: "scanning"}}
}

func panConcurrencyFixture(t *testing.T) (*Service, *panStub) {
	t.Helper()
	lib, _, _ := libraryFixture(t)
	return lib, stubOf(t, lib.drive)
}

func fixtureVideo(id, name string) scan.Video {
	return scan.IdentifyVideo(pan.File{ID: id, ParentID: "10", Name: name, Size: 1 << 30})
}

func identifyScanVideosForTest(ctx context.Context, lib *Service, payload scan.Payload, entries []pan.File) (map[string]scan.Video, error) {
	previous, err := lib.database.File.Query().WithMovie().All(ctx)
	if err != nil {
		return nil, err
	}
	known := make(map[string]*ent.File, len(previous))
	for _, entry := range previous {
		known[entry.FileID] = entry
	}
	var videos []scan.Video
	for _, entry := range entries {
		if !entry.IsDirectory && domain.IsVideo(entry.Name) {
			videos = append(videos, scan.Video{File: entry})
		}
	}
	scan.IdentifyScanVideos(payload, videos, known)
	result := make(map[string]scan.Video, len(videos))
	for _, video := range videos {
		result[video.ID] = video
	}
	return result, nil
}

func indexScanPage(ctx context.Context, lib *Service, taskID int, scanID, directoryPath string, videos []scan.Video, payload *scan.Payload) error {
	return scan.ProcessScanPage(ctx, lib.database, taskID, scanID, directoryPath, videos, payload, nil, lib.tasks)
}

func reconcileScan(ctx context.Context, lib *Service, taskID int, scanID string, payload *scan.Payload, observed scrapePkg.DirectoryObservations) error {
	return scan.ReconcileScan(ctx, lib.database, taskID, scanID, payload, observed, lib.images, lib.tasks)
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
	timeout := time.After(3 * time.Second)
	for {
		if ready() {
			return
		}
		select {
		case <-ticker.C:
		case <-timeout:
			t.Fatal("timed out waiting for concurrent 115 condition")
		}
	}
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
