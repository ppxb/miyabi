package api

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/catalogue"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/javdb"
	"github.com/ppxb/miyabi/internal/library"
	"github.com/ppxb/miyabi/internal/maintenance"
	"github.com/ppxb/miyabi/internal/monitor"
	"github.com/ppxb/miyabi/internal/offline"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

// Golden files freeze the JSON contract the frontend depends on. Run with
// -update after an intentional wire change and review the diff.
var updateGolden = flag.Bool("update", false, "rewrite golden response files")

type goldenDiscover struct {
	CatalogueManager
}

func goldenMovie() domain.Movie {
	return domain.Movie{
		ID: "movie-exact", Code: "ABP-123", Title: "Localized title", OriginTitle: "Original title",
		ReleaseDate: "2026-08-01", Duration: 120, Rating: 4.5,
		Thumbnail: "https://media.example/thumb.jpg", Cover: "https://media.example/cover.jpg",
		PreviewImages: []domain.PreviewImage{{Thumbnail: "https://media.example/preview-thumb.jpg", Original: "https://media.example/preview.jpg"}},
		PreviewVideo:  "https://media.example/preview.m3u8",
		MagnetsCount:  3, HasSubtitle: true, HasPreview: true,
		Actors: []domain.Actor{
			{ID: "actor-1", Name: "Actor", NameZHT: "演員", Gender: "female", Avatar: "https://media.example/actor.jpg"},
			{ID: "actor-2", Name: "Actor Two", Gender: "male", Avatar: "https://media.example/actor-two.jpg"},
		},
		Tags:     []domain.Tag{{ID: "tag-1", Name: "Tag", NameZHT: "標籤", CategoryID: "category-1"}},
		Series:   &domain.Series{ID: "series-1", Name: "Series"},
		Maker:    &domain.Maker{ID: "maker-1", Name: "Maker"},
		Director: &domain.Director{ID: "director-1", Name: "Director"},
	}
}

func (goldenDiscover) Browse(context.Context, domain.BrowseOptions) ([]catalogue.Movie, error) {
	bare := domain.Movie{ID: "movie-near", Code: "ABP-124", Title: "Similar result", ReleaseDate: "2027-01-01",
		Thumbnail: "https://media.example/thumb-near.jpg", Cover: "https://media.example/cover-near.jpg",
		PreviewImages: []domain.PreviewImage{}, Actors: []domain.Actor{}, Tags: []domain.Tag{}}
	return []catalogue.Movie{
		{Movie: goldenMovie(), LibraryID: 7, State: catalogue.MovieInLibrary, ReleaseStatus: catalogue.ReleaseReleased},
		{Movie: bare, State: catalogue.MovieNotInLibrary, ReleaseStatus: catalogue.ReleaseUpcoming},
	}, nil
}

func (stub goldenDiscover) Search(context.Context, string, domain.SearchOptions) ([]catalogue.Movie, error) {
	return stub.Browse(context.Background(), domain.BrowseOptions{})
}

func (goldenDiscover) MovieDetail(context.Context, string) (catalogue.MovieDetail, error) {
	return catalogue.MovieDetail{
		Movie:         catalogue.Movie{Movie: goldenMovie(), State: catalogue.MovieSaving, ReleaseStatus: catalogue.ReleaseReleased},
		Zone:          domain.ZoneCensored,
		ActorMovies:   []domain.MovieReference{{ID: "actor-movie-1", Code: "ABP-124", Thumbnail: "https://media.example/actor-movie.jpg"}},
		RelatedMovies: []domain.MovieReference{{ID: "related-movie-1", Code: "SONE-001", Thumbnail: "https://media.example/related-movie.jpg"}},
	}, nil
}

func (goldenDiscover) MovieStates(context.Context, []catalogue.MovieIdentity) ([]catalogue.MovieStateItem, error) {
	return []catalogue.MovieStateItem{
		{ID: "movie-exact", LibraryID: 7, State: catalogue.MovieInLibrary},
		{ID: "movie-near", State: catalogue.MovieNotInLibrary},
		{ID: "movie-saving", State: catalogue.MovieSaving},
	}, nil
}

func (goldenDiscover) Magnets(context.Context, string) ([]catalogue.Magnet, error) {
	return []catalogue.Magnet{
		{Magnet: domain.Magnet{Hash: "0000000000000000000000000000000000000003", Name: "HD subtitle fixture", Size: 1024 << 20,
			HasSubtitle: true, HD: true, FilesCount: 3, CreatedAt: "2026-08-03",
			Sources: []string{"javdb", "javbus"}, Tags: []string{"字幕", "高清"}}, URI: "magnet:?xt=urn:btih:0000000000000000000000000000000000000003"},
		{Magnet: domain.Magnet{Hash: "0000000000000000000000000000000000000002", Name: "HD fixture", Size: 16384 << 20,
			HD: true, FilesCount: 2, CreatedAt: "2026-08-02",
			Sources: []string{"javbus"}, Tags: []string{"高清", "4K"}, Inferred: true}, URI: "magnet:?xt=urn:btih:0000000000000000000000000000000000000002"},
	}, nil
}

func (goldenDiscover) Tags(context.Context, domain.Zone) ([]domain.TagCategory, error) {
	return []domain.TagCategory{{ID: "category-1", Name: "主題", Tags: []domain.TagOption{{ID: "tag-1", Name: "Tag"}, {ID: "tag-2", Name: "Other"}}}}, nil
}

func (goldenDiscover) Route() catalogue.RouteStatus {
	return catalogue.RouteStatus{Host: "https://api.example", LatencyMS: 125, Active: true, Manual: false,
		Candidates: []catalogue.RouteCandidate{
			{Host: "https://api.example", LatencyMS: 125, Status: javdb.RouteAvailable},
			{Host: "https://backup.example", LatencyMS: 0, Status: javdb.RouteUnavailable},
			{Host: "https://untested.example", Status: javdb.RouteUntested},
		}}
}

type goldenLibrary struct {
	LibraryManager
}

func goldenSource() domain.LibrarySource {
	return domain.LibrarySource{AccountID: "100", Directory: domain.LibraryDirectory{ID: "10", Name: "Movies", Path: "/Movies"}}
}

func goldenTime() time.Time { return time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC) }

func (goldenLibrary) Movies(context.Context, int, int) (library.Page, error) {
	source := goldenSource()
	cover, poster, javdbID := "/api/library/artwork/aa.jpg", "/api/library/artwork/bb.jpg", "movie-exact"
	return library.Page{Source: &source, Total: 2, Page: 1, HasMore: false, Movies: []library.Movie{
		{ID: 7, Code: "ABP-123", Title: "Localized title", JavDBID: &javdbID, Cover: &cover, Poster: &poster, Fanart: "/api/library/artwork/cc.jpg",
			ReleaseDate: "2026-08-01", Duration: 120, Rating: 4.5,
			Director: &library.Entity{ID: "director-1", Name: "Director"}, Maker: &library.Entity{ID: "maker-1", Name: "Maker"},
			Series: &library.Entity{ID: "series-1", Name: "Series"},
			Actors: []library.Entity{{ID: "actor-1", Name: "Actor"}}, Tags: []library.Tag{{ID: 3, JavDBID: "tag-1", Name: "Tag"}},
			ScrapeStatus: movie.ScrapeStatusDone},
		{ID: 8, Code: "ZZZ-999", Actors: []library.Entity{}, Tags: []library.Tag{}, ScrapeStatus: movie.ScrapeStatusFailed},
	}}, nil
}

func (goldenLibrary) ViewedMovieIDs(context.Context, ...int) ([]string, error) {
	return []string{"movie-exact"}, nil
}

func (goldenLibrary) AddViewedMovieIDs(context.Context, []string) error {
	return nil
}

type goldenTasks struct {
	TaskManager
}

func (goldenTasks) List(context.Context) ([]tasks.TaskInfo, error) {
	failure := "JavDB 返回的番号不一致"
	return []tasks.TaskInfo{
		{ID: 3, Type: "scan", Status: task.StatusRunning, Progress: 50, CreatedAt: goldenTime(), UpdatedAt: goldenTime().Add(time.Minute),
			Source: goldenSource(), Scan: domain.ScanProgress{Stage: "scraping", CurrentPath: "/Movies", DirectoriesDiscovered: 2, DirectoriesScanned: 2,
				FilesScanned: 5, VideoFiles: 2, MatchedFiles: 2, Movies: 2, MetadataTotal: 2, MetadataCompleted: 1}},
		{ID: 2, Type: "scan", Status: task.StatusFailed, Error: &failure, CreatedAt: goldenTime(), UpdatedAt: goldenTime(),
			Source: goldenSource(), Scan: domain.ScanProgress{Stage: "done"}, OfflineTaskID: 9},
	}, nil
}

func (goldenTasks) Revisions() tasks.TaskRevisions {
	return tasks.TaskRevisions{Library: 4, Offline: 2, Monitor: 3}
}

type goldenOffline struct {
	OfflineManager
}

func (goldenOffline) Activity(context.Context) (offline.Activity, error) {
	source := goldenSource()
	failure := "115 离线任务失败"
	return offline.Activity{Source: &source, Tasks: []domain.OfflineSubmission{
		{TaskID: 9, Code: "ABP-123", JavDBID: "movie-exact", LibraryID: 7, AccountID: "100", DirectoryID: "10", ScanTaskID: 2,
			Hash: "0000000000000000000000000000000000000003", Status: string(task.StatusDone), Phase: "in_library", Progress: 100},
		{TaskID: 10, Code: "SONE-001", JavDBID: "related-movie-1", AccountID: "100", DirectoryID: "10",
			Hash: "0000000000000000000000000000000000000002", Status: string(task.StatusRunning), Phase: "downloading", Processing: true, Progress: 42},
		{TaskID: 11, Code: "ZZZ-999", JavDBID: "movie-missing", AccountID: "100", DirectoryID: "10",
			Hash: "0000000000000000000000000000000000000001", Status: string(task.StatusFailed), Phase: "available", Error: &failure},
	}}, nil
}

type goldenSubscription struct {
	SubscriptionManager
}

func (goldenSubscription) List(context.Context, string, int, int) ([]monitor.Item, error) {
	taskID, failure, next := 9, "JavDB 暂不可用", goldenTime().Add(time.Hour)
	last := goldenTime()
	return []monitor.Item{
		{ID: 2, Kind: "movie", TargetID: "movie-upcoming", Code: "SONE-002", Title: "Upcoming", Cover: "https://media.example/upcoming.jpg", ReleaseDate: "2026-10-01",
			AutoDownload: true, Status: monitor.StatusWaiting, NextCheckAt: &next, LastCheckedAt: &last, Checks: 3, Error: &failure, CreatedAt: goldenTime(), UpdatedAt: goldenTime()},
		{ID: 1, Kind: "movie", TargetID: "movie-exact", Code: "ABP-123", Title: "Localized title", Cover: "https://media.example/cover.jpg", ReleaseDate: "2026-08-01",
			AutoDownload: true, Status: monitor.StatusAdded, Hash: "0000000000000000000000000000000000000003", TaskID: &taskID, Checks: 1, CreatedAt: goldenTime(), UpdatedAt: goldenTime()},
	}, nil
}

type goldenPan struct {
	DriveManager
}

func (goldenPan) Account(context.Context) (drive.AccountStatus, error) {
	directory := goldenSource().Directory
	return drive.AccountStatus{Connected: true, Directory: &directory, Account: &pan.Account{
		ID: "100", Name: "fixture", Avatar: "https://avatar.example/100.png", Level: "vip",
		Space: pan.AccountSpace{Total: pan.SpaceAmount{Bytes: 1 << 40, Formatted: "1TB"}, Used: pan.SpaceAmount{Bytes: 1 << 30, Formatted: "1GB"}, Remaining: pan.SpaceAmount{Bytes: (1 << 40) - (1 << 30), Formatted: "1023GB"}},
	}}, nil
}

type goldenData struct {
	MaintenanceManager
}

func (goldenData) Info(context.Context) (maintenance.Info, error) {
	return maintenance.Info{DataDirectory: "/app/data", DatabaseSizeBytes: 4 << 20,
		Cache: mediaimage.CacheStats{SizeBytes: 3 << 20, EntryCount: 12, UnusedSizeBytes: 1 << 20, UnusedEntryCount: 2}}, nil
}

func goldenRouter() http.Handler {
	return NewRouter(Dependencies{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Catalogue: goldenDiscover{}, Library: goldenLibrary{},
		Tasks: goldenTasks{}, Offline: goldenOffline{}, Monitor: goldenSubscription{}, Drive: goldenPan{}, Maintenance: goldenData{},
	})
}

func TestResponseContractsMatchGoldenFiles(t *testing.T) {
	router := goldenRouter()
	for _, scenario := range []struct {
		name, method, path string
		body               string
	}{
		{name: "discover_browse", method: http.MethodGet, path: "/api/discover/movies?zone=censored&page=1"},
		{name: "discover_search", method: http.MethodGet, path: "/api/discover/search?q=ABP-123"},
		{name: "discover_movie", method: http.MethodGet, path: "/api/discover/movies/movie-exact"},
		{name: "discover_magnets", method: http.MethodGet, path: "/api/discover/movies/movie-exact/magnets"},
		{name: "discover_movie_states", method: http.MethodPost, path: "/api/discover/movie-states",
			body: `{"movies":[{"id":"movie-exact","code":"ABP-123"},{"id":"movie-near","code":"ABP-124"},{"id":"movie-saving","code":"SONE-001"}]}`},
		{name: "discover_tags", method: http.MethodGet, path: "/api/discover/tags?zone=censored"},
		{name: "javdb_route", method: http.MethodGet, path: "/api/javdb/route"},
		{name: "library_movies", method: http.MethodGet, path: "/api/library/movies"},
		{name: "tasks", method: http.MethodGet, path: "/api/tasks"},
		{name: "offline_tasks", method: http.MethodGet, path: "/api/offline/tasks"},
		{name: "subscriptions", method: http.MethodGet, path: "/api/subscriptions"},
		{name: "pan_account", method: http.MethodGet, path: "/api/pan/account"},
		{name: "settings_system", method: http.MethodGet, path: "/api/settings/system"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var body io.Reader
			if scenario.body != "" {
				body = bytes.NewBufferString(scenario.body)
			}
			request := httptest.NewRequest(scenario.method, scenario.path, body)
			if scenario.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", response.Code, response.Body)
			}
			assertGolden(t, filepath.Join("testdata", "golden", scenario.name+".json"), response.Body.Bytes())
		})
	}
}

func assertGolden(t *testing.T, name string, body []byte) {
	t.Helper()
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err != nil {
		t.Fatalf("response is not JSON: %v\n%s", err, body)
	}
	pretty.WriteByte('\n')
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, pretty.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("missing golden file %s (run with -update): %v", name, err)
	}
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), pretty.Bytes()) {
		t.Fatalf("response for %s changed; run `go test ./internal/api -run TestResponseContracts -update` and review the diff\n--- want\n%s\n--- got\n%s", name, want, pretty.Bytes())
	}
}
