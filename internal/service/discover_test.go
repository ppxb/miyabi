package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent/setting"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/javdb"
)

func TestNewDiscoverServicePersistsDeviceWithoutSelectingRoute(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	first, err := NewDiscoverService(t.Context(), store.Client, javdb.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if status := first.Route(); status.Active || status.Host != "" {
		t.Fatalf("new service selected a route: %#v", status)
	}
	first.Close()

	record, err := store.Client.Setting.Query().Where(setting.Key(javdbDeviceSetting)).Only(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var firstDevice string
	if err := json.Unmarshal(record.Value, &firstDevice); err != nil {
		t.Fatal(err)
	}
	if _, err := uuid.Parse(firstDevice); err != nil {
		t.Fatalf("device UUID = %q: %v", firstDevice, err)
	}

	second, err := NewDiscoverService(t.Context(), store.Client, javdb.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	record, err = store.Client.Setting.Query().Where(setting.Key(javdbDeviceSetting)).Only(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var secondDevice string
	if err := json.Unmarshal(record.Value, &secondDevice); err != nil {
		t.Fatal(err)
	}
	if secondDevice != firstDevice {
		t.Fatalf("device UUID changed from %q to %q", firstDevice, secondDevice)
	}
}

func TestNewDiscoverServiceRestoresPersistedRoute(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	saved := persistedRoute{Host: "https://cached.example", LatencyMS: 125, Manual: true}
	if err := saveSetting(t.Context(), store.Client, javdbRouteSetting, saved); err != nil {
		t.Fatal(err)
	}
	service, err := NewDiscoverService(t.Context(), store.Client, javdb.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	status := service.Route()
	if !status.Active || status.Host != saved.Host || status.LatencyMS != saved.LatencyMS || status.Manual != saved.Manual {
		t.Fatalf("restored route = %#v", status)
	}
	if err := service.persistActiveRoute(t.Context()); err != nil {
		t.Fatal(err)
	}
	restored, found, err := loadSetting[persistedRoute](t.Context(), store.Client, javdbRouteSetting)
	if err != nil || !found || restored != saved {
		t.Fatalf("persisted route = %#v, found = %t, error = %v", restored, found, err)
	}
}

func TestProjectMoviesAddsLibraryTaskAndReleaseState(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	localMovie, err := store.Client.Movie.Create().SetCode("ABP-001").Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Client.File.Create().SetFileID("1001").SetName("ABP-001.mp4").SetSize(1).
		SetAccountID("100").SetRootID("10").SetMovie(localMovie).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := saveSetting(t.Context(), store.Client, panDirectorySetting, panLibraryDirectory{
		AccountID: "100", PanLibraryDirectory: PanLibraryDirectory{ID: "10", Path: "/Movies"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Client.Task.Create().
		SetType("offline").
		SetPayload(map[string]any{"code": "ABP-002"}).
		Save(t.Context()); err != nil {
		t.Fatal(err)
	}

	for _, item := range []struct {
		kind    string
		status  task.Status
		payload map[string]any
	}{
		{"scrape", task.StatusRunning, map[string]any{"code": "ABP-003"}},
		{"offline", task.StatusDone, map[string]any{"code": "ABP-003"}},
		{"offline", task.StatusFailed, map[string]any{"code": "ABP-003"}},
		{"offline", task.StatusQueued, map[string]any{"code": "ABP-999"}},
		{"offline", task.StatusQueued, map[string]any{"code": 123}},
	} {
		if err := store.Client.Task.Create().SetType(item.kind).SetStatus(item.status).SetPayload(item.payload).Exec(t.Context()); err != nil {
			t.Fatal(err)
		}
	}

	today := time.Now().In(time.Local)
	tomorrow := today.AddDate(0, 0, 1).Format("2006-01-02")
	service := &DiscoverService{database: store.Client}
	movies, err := service.projectMovies(t.Context(), []javdb.Movie{
		{ID: "one", Code: "ABP-001", ReleaseDate: today.Format("2006-01-02")},
		{ID: "two", Code: "ABP-002", ReleaseDate: tomorrow},
		{ID: "three", Code: "ABP-003"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if movies[0].State != MovieInLibrary || movies[0].ReleaseStatus != ReleaseReleased {
		t.Fatalf("library movie = %#v", movies[0])
	}
	if movies[1].State != MovieSaving || movies[1].ReleaseStatus != ReleaseUpcoming {
		t.Fatalf("saving movie = %#v", movies[1])
	}
	if movies[2].State != MovieNotInLibrary || movies[2].ReleaseStatus != ReleaseUnknown {
		t.Fatalf("remote movie = %#v", movies[2])
	}
}

func TestProjectEmptyMoviesSkipsDatabase(t *testing.T) {
	service := &DiscoverService{}
	result, err := service.projectMovies(t.Context(), nil)
	if err != nil || result == nil || len(result) != 0 {
		t.Fatalf("empty projection = %#v, error = %v", result, err)
	}
}

func TestCachedCatalogueStillReflectsCurrentLibraryAndTaskState(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service, err := NewDiscoverService(t.Context(), store.Client, javdb.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	loads := 0
	project := func() MovieState {
		t.Helper()
		source, err := cachedJavDB(t.Context(), service, service.lists, "fixture", func(context.Context) ([]javdb.Movie, error) {
			loads++
			return []javdb.Movie{{ID: "fixture", Code: "ABP-001"}}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		movies, err := service.projectMovies(t.Context(), source)
		if err != nil {
			t.Fatal(err)
		}
		return movies[0].State
	}
	if got := project(); got != MovieNotInLibrary {
		t.Fatalf("initial state = %s", got)
	}
	if err := store.Client.Task.Create().SetType("offline").SetPayload(map[string]any{"code": "ABP-001"}).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := project(); got != MovieSaving {
		t.Fatalf("queued state = %s", got)
	}
	localMovie, err := store.Client.Movie.Create().SetCode("ABP-001").Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got := project(); got != MovieSaving {
		t.Fatalf("metadata without a file changed library state: %s", got)
	}
	if err := store.Client.File.Create().SetFileID("1001").SetName("ABP-001.mp4").SetSize(1).
		SetAccountID("100").SetRootID("10").SetMovie(localMovie).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := saveSetting(t.Context(), store.Client, panDirectorySetting, panLibraryDirectory{
		AccountID: "100", PanLibraryDirectory: PanLibraryDirectory{ID: "10", Path: "/Movies"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := project(); got != MovieInLibrary || loads != 1 {
		t.Fatalf("scanned state = %s, loads = %d", got, loads)
	}
	if err := saveSetting(t.Context(), store.Client, panDirectorySetting, panLibraryDirectory{
		AccountID: "100", PanLibraryDirectory: PanLibraryDirectory{ID: "20", Path: "/Other"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := project(); got != MovieSaving || loads != 1 {
		t.Fatalf("movie outside the mounted root = %s, loads = %d", got, loads)
	}
}

func TestProjectMagnetsBuildsStandardURIFromHash(t *testing.T) {
	const hash = "0000000000000000000000000000000000000001"
	result := projectMagnets([]javdb.Magnet{{Hash: hash, Name: "Fixture", Size: 1024}})
	if len(result) != 1 || result[0].URI != "magnet:?xt=urn:btih:"+hash || result[0].Name != "Fixture" {
		t.Fatalf("result = %#v", result)
	}
	if empty := projectMagnets(nil); empty == nil || len(empty) != 0 {
		t.Fatalf("empty result = %#v", empty)
	}
}
