package catalogue

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/setting"
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

	first, err := New(t.Context(), store.Client, nil, nil)
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

	second, err := New(t.Context(), store.Client, nil, nil)
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
	service, err := New(t.Context(), store.Client, nil, nil)
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

func TestProjectMoviesOmitsInvalidDatesWithoutMutatingCatalogue(t *testing.T) {
	now := time.Now()
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
	result := projectMovies(t.Context(), source)
	if len(result) != len(source) {
		t.Fatalf("optional dates blocked the page: %#v", result)
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
	result := projectMovies(t.Context(), nil)
	if result == nil || len(result) != 0 {
		t.Fatalf("empty projection = %#v", result)
	}
}

func TestMovieStatesUsesSourceIDBeforeCatalogueSpelling(t *testing.T) {
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

	store.Client.OfflineDownload.Create().SetHash("fixture-hash").SetJavdbID("queued-id").SetCode("PREVIOUS-002").
		SetAccountID(source.AccountID).SetDirectoryID(source.Directory.ID).SaveX(ctx)

	service := &Service{database: store.Client, local: localState}
	sourceMovies := []domain.Movie{
		{ID: "known-id", Code: "作品/新版 #001"},
		{ID: "pending-id", Code: "knb_m014"},
		{ID: "conflicting-id", Code: "GLOD-0436"},
		{ID: "queued-id", Code: "Current.Number"},
	}
	identities := make([]MovieIdentity, len(sourceMovies))
	for i, item := range sourceMovies {
		identities[i] = MovieIdentity{ID: item.ID, Code: item.Code}
	}
	result, err := service.MovieStates(ctx, identities)
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

type stubProviderWithMagnets struct {
	magnets []domain.Magnet
	movies  []domain.Movie
}

func (s *stubProviderWithMagnets) Close() {}
func (s *stubProviderWithMagnets) Search(context.Context, string, domain.SearchOptions) ([]domain.Movie, error) {
	return s.movies, nil
}
func (s *stubProviderWithMagnets) Browse(context.Context, domain.BrowseOptions) ([]domain.Movie, error) {
	return s.movies, nil
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

	const hash = "abcdef0123456789abcdef0123456789abcdef01"
	provider := &stubProviderWithMagnets{
		magnets: []domain.Magnet{
			{
				Hash:        hash,
				Name:        "SSIS-001 Subtitle",
				Size:        2000,
				HasSubtitle: true,
				Sources:     []string{"javdb"},
				Tags:        []string{domain.MagnetTagSubtitle},
			},
			{
				Hash:    hash,
				Name:    "duplicate from primary source",
				Sources: []string{domain.MagnetSourceJavDB},
			},
		},
	}

	service, err := NewWithClients(t.Context(), store.Client, provider, nil, &stubLocalState{})
	if err != nil {
		t.Fatalf("unexpected error creating service: %v", err)
	}
	defer service.Close()

	magnets, err := service.Magnets(t.Context(), "movie-1")
	if err != nil {
		t.Fatalf("unexpected error fetching magnets: %v", err)
	}
	if len(magnets) != 1 {
		t.Fatalf("expected 1 magnet, got %d", len(magnets))
	}
	if magnets[0].URI != "magnet:?xt=urn:btih:"+hash {
		t.Errorf("unexpected magnet URI: %s", magnets[0].URI)
	}

	want := magnets[0].Magnet
	// All entry points must share the populated cache, even when the source changes.
	provider.magnets = nil
	magnets[0] = Magnet{}
	domainMagnets, err := service.CatalogueMagnets(t.Context(), "movie-1")
	if err != nil || len(domainMagnets) != 1 || !reflect.DeepEqual(domainMagnets[0], want) {
		t.Fatalf("domain magnets = %+v, error = %v, want cached %+v", domainMagnets, err, want)
	}
	domainMagnets[0] = domain.Magnet{}
	for _, lookup := range []struct {
		hash string
		want bool
	}{
		{hash: strings.ToUpper(hash), want: true},
		{hash: "0000000000000000000000000000000000000000", want: false},
	} {
		has, err := service.HasMagnet(t.Context(), "movie-1", lookup.hash)
		if err != nil || has != lookup.want {
			t.Errorf("HasMagnet(%q) = %v, error = %v, want %v", lookup.hash, has, err, lookup.want)
		}
	}
	cached, err := service.Magnets(t.Context(), "movie-1")
	if err != nil || len(cached) != 1 || !reflect.DeepEqual(cached[0].Magnet, want) || cached[0].URI != "magnet:?xt=urn:btih:"+hash {
		t.Fatalf("returned slices changed cached magnets: %+v, error = %v", cached, err)
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
	service, err := NewWithClients(t.Context(), store.Client, provider, nil, &stubLocalState{})
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
