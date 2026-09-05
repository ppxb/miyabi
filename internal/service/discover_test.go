package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent/setting"
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

func TestProjectMoviesAddsLibraryTaskAndReleaseState(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.Client.Movie.Create().SetCode("ABP-001").Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Client.Task.Create().
		SetType("offline").
		SetPayload(map[string]any{"code": "abp002"}).
		Save(t.Context()); err != nil {
		t.Fatal(err)
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
