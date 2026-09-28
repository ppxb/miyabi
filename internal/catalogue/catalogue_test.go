package catalogue

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/setting"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/javdb"
	"github.com/ppxb/miyabi/internal/tasks"
)

type stubLocalState struct {
	source *domain.LibrarySource
	movies []domain.LocalMovie
}

func (s *stubLocalState) Source() *domain.LibrarySource {
	return s.source
}

func (s *stubLocalState) MatchingMovies(_ context.Context, ids, codes []string) ([]domain.LocalMovie, error) {
	var matches []domain.LocalMovie
	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}
	codeSet := make(map[string]bool)
	for _, code := range codes {
		codeSet[code] = true
	}
	for _, m := range s.movies {
		if m.JavDBID != nil && idSet[*m.JavDBID] {
			matches = append(matches, m)
		} else if m.JavDBID == nil && codeSet[codeid.Normalize(m.Code)] {
			matches = append(matches, m)
		}
	}
	return matches, nil
}

func taskPayloadJSON(t testing.TB, value any) json.RawMessage {
	t.Helper()
	payload, err := tasks.EncodePayload(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestNewPersistsDeviceWithoutSelectingRoute(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	first, err := New(t.Context(), store.Client, javdb.Options{}, nil, nil)
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

	second, err := New(t.Context(), store.Client, javdb.Options{}, nil, nil)
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

func TestNewRestoresPersistedRoute(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	saved := persistedRoute{Host: "https://cached.example", LatencyMS: 125, Manual: true}
	if err := database.SaveSetting(t.Context(), store.Client, javdbRouteSetting, saved); err != nil {
		t.Fatal(err)
	}
	service, err := New(t.Context(), store.Client, javdb.Options{}, nil, nil)
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
	restored, found, err := database.LoadSetting[persistedRoute](t.Context(), store.Client, javdbRouteSetting)
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

	source := domain.LibrarySource{
		AccountID: "100", Directory: domain.LibraryDirectory{ID: "10", Name: "Movies", Path: "/Movies"},
	}
	localState := &stubLocalState{
		source: &source,
		movies: []domain.LocalMovie{{ID: 42, Code: "ABP-001"}},
	}

	if _, err := store.Client.Task.Create().
		SetType("offline").
		SetPayload(taskPayloadJSON(t, map[string]any{"code": "ABP-002", "javdb_id": "two", "account_id": "100", "directory_id": "10"})).
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
		if err := store.Client.Task.Create().SetType(item.kind).SetStatus(item.status).SetPayload(taskPayloadJSON(t, item.payload)).Exec(t.Context()); err != nil {
			t.Fatal(err)
		}
	}

	today := time.Now().In(time.Local)
	tomorrow := today.AddDate(0, 0, 1).Format("2006-01-02")
	service := &Service{database: store.Client, local: localState}
	movies, err := service.projectMovies(t.Context(), []domain.Movie{
		{ID: "one", Code: "ABP-001", ReleaseDate: today.Format("2006-01-02")},
		{ID: "two", Code: "ABP-002", ReleaseDate: tomorrow},
		{ID: "three", Code: "ABP-003"},
		{ID: "one", Code: "ABP-001", ReleaseDate: "2026-02-30"},
		{ID: "two", Code: "ABP-002", ReleaseDate: "TBA"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if movies[0].State != MovieInLibrary || movies[0].ReleaseStatus != ReleaseReleased || movies[0].LibraryID != 42 {
		t.Fatalf("library movie = %#v", movies[0])
	}
	if movies[1].State != MovieSaving || movies[1].ReleaseStatus != ReleaseUpcoming {
		t.Fatalf("saving movie = %#v", movies[1])
	}
	if movies[2].State != MovieNotInLibrary || movies[2].ReleaseStatus != ReleaseUnknown {
		t.Fatalf("remote movie = %#v", movies[2])
	}
	if movies[3].State != MovieInLibrary || movies[3].ReleaseStatus != ReleaseUnknown ||
		movies[3].ReleaseDate != "" || movies[3].LibraryID != 42 {
		t.Fatalf("invalid date changed library state: %#v", movies[3])
	}
	if movies[4].State != MovieSaving || movies[4].ReleaseStatus != ReleaseUnknown || movies[4].ReleaseDate != "" {
		t.Fatalf("invalid date changed task state: %#v", movies[4])
	}
}

func TestProjectMoviesOmitsInvalidDatesWithoutMutatingCatalogue(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := &Service{database: store.Client}
	now := time.Now().In(time.Local)
	today := now.Format("2006-01-02")
	past := now.AddDate(0, 0, -1).Format("2006-01-02")
	future := now.AddDate(0, 0, 1).Format("2006-01-02")
	cases := []struct {
		input  string
		date   string
		status ReleaseStatus
	}{
		{past, past, ReleaseReleased},
		{"", "", ReleaseUnknown},
		{" \t\n", "", ReleaseUnknown},
		{"2026-02-30", "", ReleaseUnknown},
		{"0000-00-00", "", ReleaseUnknown},
		{"TBA", "", ReleaseUnknown},
		{"2026-09", "", ReleaseUnknown},
		{"2026-09-10T00:00:00Z", "", ReleaseUnknown},
		{" " + today + " ", today, ReleaseReleased},
		{future, future, ReleaseUpcoming},
	}
	source := make([]domain.Movie, len(cases))
	for index, test := range cases {
		source[index] = domain.Movie{
			ID: fmt.Sprintf("movie-%d", index), Code: fmt.Sprintf("ABP-%03d", index),
			Title: "Fixture title", ReleaseDate: test.input,
		}
	}
	result, err := service.projectMovies(t.Context(), source)
	if err != nil || len(result) != len(source) {
		t.Fatalf("optional dates blocked the page: %#v, %v", result, err)
	}
	for index, test := range cases {
		movie := result[index]
		if movie.ReleaseDate != test.date || movie.ReleaseStatus != test.status ||
			movie.ID != source[index].ID || movie.Code != source[index].Code || movie.Title != source[index].Title {
			t.Errorf("date %q damaged movie projection: %#v", test.input, movie)
		}
		if source[index].ReleaseDate != test.input {
			t.Errorf("projection mutated cached catalogue date %q to %q", test.input, source[index].ReleaseDate)
		}
	}
}

func TestProjectEmptyMoviesSkipsDatabase(t *testing.T) {
	service := &Service{}
	result, err := service.projectMovies(t.Context(), nil)
	if err != nil || result == nil || len(result) != 0 {
		t.Fatalf("empty projection = %#v, error = %v", result, err)
	}
}

func TestProjectionUsesSourceIDBeforeCatalogueSpelling(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := t.Context()

	source := domain.LibrarySource{
		AccountID: "100", Directory: domain.LibraryDirectory{ID: "10", Name: "Movies", Path: "/Movies"},
	}
	knownID := "known-id"
	differentID := "different-id"
	localState := &stubLocalState{
		source: &source,
		movies: []domain.LocalMovie{
			{ID: 10, Code: "OLD-001", JavDBID: &knownID},
			{ID: 11, Code: "KNB-M014", JavDBID: nil},
			{ID: 12, Code: "GLOD-0436", JavDBID: &differentID},
		},
	}

	store.Client.Task.Create().SetType("offline").SetPayload(taskPayloadJSON(t, map[string]any{
		"javdb_id": "queued-id", "code": "PREVIOUS-002",
		"account_id": source.AccountID, "directory_id": source.Directory.ID,
	})).SaveX(ctx)

	service := &Service{database: store.Client, local: localState}
	sourceMovies := []domain.Movie{
		{ID: "known-id", Code: "作品/新版 #001"},
		{ID: "pending-id", Code: "knb_m014"},
		{ID: "conflicting-id", Code: "GLOD-0436"},
		{ID: "queued-id", Code: "Current.Number"},
	}
	result, err := service.projectMovies(ctx, sourceMovies)
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []int{10, 11, 0, 0}
	wantStates := []MovieState{MovieInLibrary, MovieInLibrary, MovieNotInLibrary, MovieSaving}
	for i, item := range result {
		if item.LibraryID != wantIDs[i] || item.State != wantStates[i] {
			t.Fatalf("item %d: got id=%d state=%s, want id=%d state=%s", i, item.LibraryID, item.State, wantIDs[i], wantStates[i])
		}
	}
}

func TestFacets(t *testing.T) {
	service := &Service{}
	facets := service.Facets()
	if len(facets.Zones) == 0 || len(facets.Sorts) == 0 {
		t.Fatalf("empty facets: %+v", facets)
	}
	expectedZones := map[string]bool{"censored": true, "uncensored": true, "fc2": true, "western": true, "anime": true}
	for _, z := range facets.Zones {
		if !expectedZones[z.Value] {
			t.Errorf("unexpected zone: %s", z.Value)
		}
	}
	expectedSorts := map[string]bool{"release": true, "update": true, "hit": true, "score": true}
	for _, s := range facets.Sorts {
		if !expectedSorts[s.Value] {
			t.Errorf("unexpected sort: %s", s.Value)
		}
	}
}

type stubProviderWithMagnets struct {
	magnets []domain.Magnet
}

func (s *stubProviderWithMagnets) Close() {}
func (s *stubProviderWithMagnets) Search(context.Context, string, domain.SearchOptions) ([]domain.Movie, error) {
	return nil, nil
}
func (s *stubProviderWithMagnets) Browse(context.Context, domain.BrowseOptions) ([]domain.Movie, error) {
	return nil, nil
}
func (s *stubProviderWithMagnets) MovieDetail(context.Context, string) (domain.MovieDetail, error) {
	return domain.MovieDetail{Movie: domain.Movie{ID: "movie-1", Code: "SSIS-001"}}, nil
}
func (s *stubProviderWithMagnets) Magnets(context.Context, string) ([]domain.Magnet, error) {
	return s.magnets, nil
}
func (s *stubProviderWithMagnets) FetchMedia(context.Context, string) (domain.Media, error) {
	return domain.Media{}, nil
}
func (s *stubProviderWithMagnets) Tags(context.Context, domain.Zone) ([]domain.TagCategory, error) {
	return nil, nil
}
func (s *stubProviderWithMagnets) ResolveMovieID(context.Context, string) (string, error) {
	return "movie-1", nil
}
func (s *stubProviderWithMagnets) Route() (javdb.RouteStatus, bool) {
	return javdb.RouteStatus{}, false
}
func (s *stubProviderWithMagnets) SelectRoute(context.Context, string) (javdb.RouteStatus, error) {
	return javdb.RouteStatus{}, nil
}
func (s *stubProviderWithMagnets) Reselect(context.Context) (javdb.RouteStatus, error) {
	return javdb.RouteStatus{}, nil
}
func (s *stubProviderWithMagnets) Name() string { return "javdb" }
func (s *stubProviderWithMagnets) Find(ctx context.Context, ref domain.MovieRef) ([]domain.Magnet, error) {
	return s.magnets, nil
}

func TestServiceMagnetsWithAggregator(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	provider := &stubProviderWithMagnets{
		magnets: []domain.Magnet{
			{
				Hash:        "1111111111111111111111111111111111111111",
				Name:        "SSIS-001 Subtitle",
				Size:        2000,
				HasSubtitle: true,
				Sources:     []string{"javdb"},
			},
		},
	}

	service, err := NewWithProvider(t.Context(), store.Client, provider, &stubLocalState{})
	if err != nil {
		t.Fatalf("unexpected error creating service: %v", err)
	}

	magnets, err := service.Magnets(t.Context(), "movie-1")
	if err != nil {
		t.Fatalf("unexpected error fetching magnets: %v", err)
	}
	if len(magnets) != 1 {
		t.Fatalf("expected 1 magnet, got %d", len(magnets))
	}
	if magnets[0].URI != "magnet:?xt=urn:btih:1111111111111111111111111111111111111111" {
		t.Errorf("unexpected magnet URI: %s", magnets[0].URI)
	}

	has, err := service.HasMagnet(t.Context(), "movie-1", "1111111111111111111111111111111111111111")
	if err != nil || !has {
		t.Errorf("expected HasMagnet to return true, got %v, err=%v", has, err)
	}
}

type stubProviderWithRoute struct {
	stubProviderWithMagnets
	route javdb.RouteStatus
}

func (s *stubProviderWithRoute) Route() (javdb.RouteStatus, bool) {
	return s.route, true
}

func TestCachedJavDBIgnoresPersistActiveRouteFailure(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	provider := &stubProviderWithRoute{
		route: javdb.RouteStatus{Host: "https://new-route.example"},
	}
	service, err := NewWithProvider(t.Context(), store.Client, provider, &stubLocalState{})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}

	// Close the DB store so any write in persistActiveRoute fails
	store.Close()

	cache := newResponseCache[string](2, time.Hour)
	var loadCalls int
	load := func(context.Context) (string, error) {
		loadCalls++
		return "test-result", nil
	}

	val, err := cachedJavDB(t.Context(), service, cache, "key-1", load)
	if err != nil {
		t.Fatalf("cachedJavDB should succeed even if persistActiveRoute fails, got err: %v", err)
	}
	if val != "test-result" {
		t.Fatalf("expected test-result, got %s", val)
	}
	if loadCalls != 1 {
		t.Fatalf("expected 1 load call, got %d", loadCalls)
	}

	// Second request should hit cache and not invoke load again
	val2, err := cachedJavDB(t.Context(), service, cache, "key-1", load)
	if err != nil || val2 != "test-result" || loadCalls != 1 {
		t.Fatalf("expected cached result without reload, got val=%s, calls=%d, err=%v", val2, loadCalls, err)
	}
}
