package javdb

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"golang.org/x/time/rate"
)

type fixtureCall struct {
	path     string
	params   url.Values
	language string
}

type fixtureTransport struct {
	responses map[string][]byte
	calls     []fixtureCall
}

func (transport *fixtureTransport) getJSON(
	_ context.Context,
	path string,
	params url.Values,
	language string,
	destination any,
) error {
	transport.calls = append(transport.calls, fixtureCall{path: path, params: params, language: language})
	body, ok := transport.responses[path+"|"+language]
	if !ok {
		return fmt.Errorf("missing fixture for %s in %s", path, language)
	}
	return decodeEnvelope(body, destination)
}

func (*fixtureTransport) closeIdleConnections() {}

func fixtureFile(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func clientWithTransport(transport jsonTransport) *Client {
	client := &Client{limiter: rate.NewLimiter(rate.Inf, 1)}
	client.current.Store(&routeState{transport: transport})
	return client
}

func TestSearchDecodesMoviesAndBuildsParams(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v2/search|zh-TW": fixtureFile(t, "search.json"),
	}}
	client := clientWithTransport(transport)

	movies, err := client.Search(t.Context(), " ABP-123 ", domain.SearchOptions{
		Zone:     domain.ZoneCensored,
		Sort:     "release",
		FilterBy: "magnets",
		Page:     2,
		Limit:    40,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(movies) != 2 || movies[1].ID != "movie-exact" || movies[1].Code != "ABP-123" {
		t.Fatalf("movies = %#v", movies)
	}
	if movies[1].PreviewImages[0].Original != "https://media.example/preview.jpg" {
		t.Fatalf("preview images = %#v", movies[1].PreviewImages)
	}

	call := transport.calls[0]
	if call.path != "/api/v2/search" || call.language != defaultLanguage {
		t.Fatalf("call = %#v", call)
	}
	for key, want := range map[string]string{
		"q": "ABP-123", "page": "2", "limit": "40", "type": "movie",
		"movie_type": "0", "movie_sort_by": "release", "movie_filter_by": "magnets",
	} {
		if got := call.params.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestBrowseUsesDocumentedFilterMask(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v1/movies/tags|zh-TW": fixtureFile(t, "browse.json"),
	}}
	client := clientWithTransport(transport)

	movies, err := client.Browse(t.Context(), domain.BrowseOptions{
		Zone:   domain.ZoneCensored,
		Main:   []string{"m", "c"},
		TagIDs: []string{"tag-1", "tag-2"},
		Year:   "2026",
		Month:  "9",
		Sort:   "hit",
		Order:  "desc",
		Page:   1,
		Limit:  20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(movies) != 1 || movies[0].Code != "SONE-001" {
		t.Fatalf("movies = %#v", movies)
	}

	params := transport.calls[0].params
	if got := params.Get("filter_by"); got != "0:t:m,c:tag-1,tag-2:2026:9:" {
		t.Fatalf("filter_by = %q", got)
	}
	if params.Get("sort_by") != "hit" || params.Get("order_by") != "desc" ||
		params.Get("page") != "1" || params.Get("limit") != "20" {
		t.Fatalf("params = %v", params)
	}
}

func TestBrowseWithoutZoneKeepsTheFilterMask(t *testing.T) {
	for _, test := range []struct {
		name   string
		main   []string
		tagIDs []string
		sort   string
		mask   string
	}{
		{name: "latest", main: []string{"m"}, sort: "update", mask: ":t:m::::"},
		{name: "upcoming", sort: "release", mask: ":t:::::"},
		{name: "local tag search", main: []string{"m"}, tagIDs: []string{"tag-1"}, sort: "release", mask: ":t:m:tag-1:::"},
	} {
		t.Run(test.name, func(t *testing.T) {
			params, err := buildBrowseParams(domain.BrowseOptions{
				Main: test.main, TagIDs: test.tagIDs, Sort: test.sort, Order: "desc", Page: 1, Limit: 20,
			})
			if err != nil {
				t.Fatal(err)
			}
			if params.Get("filter_by") != test.mask || params.Get("sort_by") != test.sort {
				t.Fatalf("params = %v", params)
			}
		})
	}
}

func TestBrowsePreservesWesternSceneNumbers(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v1/movies/tags|zh-TW": fixtureFile(t, "browse_western.json"),
	}}
	client := clientWithTransport(transport)
	movies, err := client.Browse(t.Context(), domain.BrowseOptions{
		Zone: domain.ZoneWestern, Sort: "release", Order: "desc", Page: 1, Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(movies) != 2 || movies[0].Code != "ExampleStudioName.26.09.05" || movies[1].Code != "ExampleStudioName.26.09.06" {
		t.Fatalf("western movies = %+v", movies)
	}
}

func TestMovieReferencesPreserveWesternSceneNumbers(t *testing.T) {
	movies := movieReferencesFromWire(t.Context(), "movie", "actor_movies", []wireMovieReference{
		{ID: "western-scene-one", Number: "ExampleStudio.26.09.05"},
		{ID: "western-scene-two", Number: "ExampleStudio.26.09.06"},
	})
	if len(movies) != 2 || movies[0].Code != "ExampleStudio.26.09.05" || movies[1].Code != "ExampleStudio.26.09.06" {
		t.Fatalf("western references = %+v", movies)
	}
}

func TestBrowsePreservesCatalogueNumberSegments(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v1/movies/tags|zh-TW": fixtureFile(t, "browse_catalogue_numbers.json"),
	}}
	client := clientWithTransport(transport)
	movies, err := client.Browse(t.Context(), domain.BrowseOptions{
		Zone: domain.ZoneUncensored, Sort: "release", Order: "desc", Page: 1, Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"SSIS-589", "FC2-PPV-1234567", "heydouga-4030-2347", "ExampleStudio.26.09.05",
		"scute-1575-itsuki", "scute-1575-nanami", "scute-15750-itsuki", "EXAMPLE-123-model2-0001",
		"ExampleStudioName-001", "SSIS-1",
	}
	if len(movies) != len(want) {
		t.Fatalf("got %d movies, want %d", len(movies), len(want))
	}
	for index, code := range want {
		if movies[index].Code != code {
			t.Errorf("movie %d code = %q, want %q", index, movies[index].Code, code)
		}
	}
}

func TestMovieDetailMapsGraphWithoutPlot(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v4/movies/movie-exact|zh-TW": fixtureFile(t, "movie.json"),
	}}
	client := clientWithTransport(transport)

	movie, err := client.MovieDetail(t.Context(), "movie-exact")
	if err != nil {
		t.Fatal(err)
	}
	if movie.Code != "ABP-123" || movie.Rating != 4.5 || movie.PreviewVideo == "" {
		t.Fatalf("movie = %#v", movie)
	}
	if len(movie.Actors) != 2 || movie.Actors[0].NameZHT != "演員" ||
		movie.Actors[0].Gender != "female" || movie.Actors[1].Gender != "male" {
		t.Fatalf("actors = %#v", movie.Actors)
	}
	if !movie.HasSubtitle || !movie.HasPreview {
		t.Fatalf("subtitle = %v, preview = %v", movie.HasSubtitle, movie.HasPreview)
	}
	if len(movie.Tags) != 1 || movie.Tags[0].CategoryID != "category-1" {
		t.Fatalf("tags = %#v", movie.Tags)
	}
	if movie.Series == nil || movie.Maker == nil || movie.Director == nil {
		t.Fatalf("graph = %#v %#v %#v", movie.Series, movie.Maker, movie.Director)
	}
	if movie.Zone != domain.ZoneCensored || len(movie.ActorMovies) != 1 || len(movie.RelatedMovies) != 1 {
		t.Fatalf("zone = %s, actor movies = %#v, related movies = %#v", movie.Zone, movie.ActorMovies, movie.RelatedMovies)
	}
	if movie.ActorMovies[0].Code != "abp124" || movie.RelatedMovies[0].Code != "SONE-001a" || movie.RelatedMovies[0].Thumbnail != "https://media.example/related-movie.jpg" {
		t.Fatalf("recommendations = %#v, %#v", movie.ActorMovies, movie.RelatedMovies)
	}
}

func TestMovieDetailPreservesMultipartNumbers(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v4/movies/movie-multipart|zh-TW": fixtureFile(t, "movie_multipart.json"),
	}}
	client := clientWithTransport(transport)
	movie, err := client.MovieDetail(t.Context(), "movie-multipart")
	if err != nil {
		t.Fatal(err)
	}
	if movie.Code != "heydouga-4030-2347" || len(movie.ActorMovies) != 6 || len(movie.RelatedMovies) != 1 {
		t.Fatalf("multipart detail = %+v", movie)
	}
	if movie.ActorMovies[0].Code != "T28-638" || movie.ActorMovies[5].Code != "heydouga-4030-2347" || movie.RelatedMovies[0].Code != "heydouga-4030-2348" {
		t.Fatalf("multipart references = %+v, %+v", movie.ActorMovies, movie.RelatedMovies)
	}
}

func TestMovieDetailPreservesNamedNumbers(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v4/movies/named-itsuki|zh-TW": fixtureFile(t, "movie_named.json"),
	}}
	client := clientWithTransport(transport)
	movie, err := client.MovieDetail(t.Context(), "named-itsuki")
	if err != nil {
		t.Fatal(err)
	}
	if movie.Code != "scute-1575-itsuki" || len(movie.ActorMovies) != 6 || len(movie.RelatedMovies) != 1 {
		t.Fatalf("named detail = %+v", movie)
	}
	if movie.ActorMovies[4].Code != "EXAMPLE-123-model2-0001" ||
		movie.ActorMovies[5].Code != "scute-15750-itsuki" || movie.RelatedMovies[0].Code != "scute-1575-nanami" {
		t.Fatalf("named references = %+v, %+v", movie.ActorMovies, movie.RelatedMovies)
	}
}

func TestBrowseBuildsEntityFilters(t *testing.T) {
	for _, test := range []struct {
		kind domain.EntityType
		want string
	}{
		{domain.EntityActor, ":a:entity-1"},
		{domain.EntitySeries, ":s:entity-1"},
		{domain.EntityMaker, ":m:entity-1"},
		{domain.EntityDirector, ":d:entity-1"},
	} {
		t.Run(string(test.kind), func(t *testing.T) {
			options := domain.BrowseOptions{EntityType: test.kind, EntityID: "entity-1", Sort: "release", Order: "desc", Page: 2, Limit: 20}
			params, err := buildBrowseParams(options)
			if err != nil {
				t.Fatal(err)
			}
			if params.Get("filter_by") != test.want || params.Get("page") != "2" || params.Get("limit") != "20" {
				t.Fatalf("params = %v", params)
			}
			options.Main = []string{"m", "c"}
			params, err = buildBrowseParams(options)
			if err != nil {
				t.Fatal(err)
			}
			if params.Get("filter_by") != test.want+":m,c::" {
				t.Fatalf("main filter = %s", params.Get("filter_by"))
			}
			options.Zone = domain.ZoneFC2
			if _, err := buildBrowseParams(options); err == nil {
				t.Fatal("accepted an unsupported zone filter for entity movies")
			}
			options.Zone = ""
			options.TagIDs = []string{"tag-1"}
			if _, err := buildBrowseParams(options); err == nil {
				t.Fatal("accepted unsupported mixed entity and tag filters")
			}
		})
	}
}

func TestResolveMovieIDRequiresExactNormalizedMatch(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v2/search|zh-TW": fixtureFile(t, "search.json"),
	}}
	client := clientWithTransport(transport)

	id, err := client.ResolveMovieID(t.Context(), "abp123")
	if err != nil {
		t.Fatal(err)
	}
	if id != "movie-exact" {
		t.Fatalf("id = %q", id)
	}
}

func TestResolveMovieIDDuplicateMatches(t *testing.T) {
	tests := []struct {
		name    string
		movies  string
		wantID  string
		wantErr string
	}{
		{
			name: "same ID with exact and normalized duplicates",
			movies: `[
				{"id":"movie-exact","number":"ABP-123"},
				{"id":"movie-similar","number":"ABP-1234"},
				{"id":"movie-exact","number":"abp123"},
				{"id":"movie-exact","number":"ABP-123"}
			]`,
			wantID: "movie-exact",
		},
		{
			name: "different ID after duplicate matches",
			movies: `[
				{"id":"movie-exact","number":"ABP-123"},
				{"id":"movie-exact","number":"ABP-123"},
				{"id":"movie-other","number":"ABP_123"}
			]`,
			wantErr: "catalogue number ABP-123 has multiple exact JavDB matches",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &fixtureTransport{responses: map[string][]byte{
				"/api/v2/search|zh-TW": []byte(`{"success":1,"data":{"movies":` + test.movies + `}}`),
			}}
			client := clientWithTransport(transport)
			id, err := client.ResolveMovieID(t.Context(), "abp123")
			if test.wantErr != "" {
				if err == nil || err.Error() != test.wantErr || id != "" {
					t.Fatalf("id = %q, error = %v; want empty ID and error %q", id, err, test.wantErr)
				}
				return
			}
			if err != nil || id != test.wantID {
				t.Fatalf("id = %q, error = %v; want %q", id, err, test.wantID)
			}
		})
	}
}

func TestResolveMovieIDFormatEquivalence(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		movies  string
		wantID  string
		wantErr string
	}{
		{
			name:  "compact input with padding zeros resolves to unpadded hyphenated",
			input: "abc00123",
			movies: `[
				{"id":"movie-abc","number":"ABC-123"},
				{"id":"movie-other","number":"ABC-1234"}
			]`,
			wantID: "movie-abc",
		},
		{
			name:  "three-digit padded input resolves to single digit sequence",
			input: "ABP-001",
			movies: `[
				{"id":"movie-abp1","number":"ABP-1"},
				{"id":"movie-abp10","number":"ABP-10"}
			]`,
			wantID: "movie-abp1",
		},
		{
			name:  "hyphenated two digit resolves from three digit padding",
			input: "IPX-52",
			movies: `[
				{"id":"movie-ipx52","number":"IPX-052"},
				{"id":"movie-ipx520","number":"IPX-520"}
			]`,
			wantID: "movie-ipx52",
		},
		{
			name:  "exact match wins over format-equivalent match",
			input: "ABC-123",
			movies: `[
				{"id":"movie-exact","number":"ABC-123"},
				{"id":"movie-equiv","number":"ABC-0123"}
			]`,
			wantID: "movie-exact",
		},
		{
			name:  "multiple distinct equivalent candidates are strictly rejected",
			input: "ABC00123",
			movies: `[
				{"id":"movie-first","number":"ABC-123"},
				{"id":"movie-second","number":"ABC-0123"}
			]`,
			wantErr: "catalogue number ABC-00123 has multiple format-equivalent JavDB matches",
		},
		{
			name:  "duplicate rows for the same equivalent ID are deduplicated and accepted",
			input: "ABC00123",
			movies: `[
				{"id":"movie-same","number":"ABC-123"},
				{"id":"movie-same","number":"ABC-123"}
			]`,
			wantID: "movie-same",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fixtureTransport{responses: map[string][]byte{
				"/api/v2/search|zh-TW": []byte(`{"success":1,"data":{"movies":` + tt.movies + `}}`),
			}}
			client := clientWithTransport(transport)
			id, err := client.ResolveMovieID(t.Context(), tt.input)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr || id != "" {
					t.Fatalf("id = %q, error = %v; want empty ID and error %q", id, err, tt.wantErr)
				}
				return
			}
			if err != nil || id != tt.wantID {
				t.Fatalf("id = %q, error = %v; want %q", id, err, tt.wantID)
			}
		})
	}
}

type queryFixtureTransport struct {
	responses map[string][]byte
	calls     []fixtureCall
}

func (transport *queryFixtureTransport) getJSON(
	_ context.Context,
	path string,
	params url.Values,
	language string,
	destination any,
) error {
	transport.calls = append(transport.calls, fixtureCall{path: path, params: params, language: language})
	q := params.Get("q")
	body, ok := transport.responses[q]
	if !ok {
		return fmt.Errorf("missing fixture for query %q", q)
	}
	return decodeEnvelope(body, destination)
}

func (*queryFixtureTransport) closeIdleConnections() {}

func TestResolveMovieIDFallbackSearch(t *testing.T) {
	// Simulate empty result when searching padded "ABC-00123", but succeeding
	// when fallback search queries unpadded "ABC-123".
	transport := &queryFixtureTransport{responses: map[string][]byte{
		"ABC-00123": []byte(`{"success":1,"data":{"movies":[]}}`),
		"ABC-123": []byte(`{"success":1,"data":{"movies":[
			{"id":"movie-fallback","number":"ABC-123"}
		]}}`),
	}}
	client := clientWithTransport(transport)
	id, err := client.ResolveMovieID(t.Context(), "ABC-00123")
	if err != nil || id != "movie-fallback" {
		t.Fatalf("id = %q, error = %v; want %q", id, err, "movie-fallback")
	}
	if len(transport.calls) != 2 {
		t.Fatalf("expected 2 calls (initial + fallback), got %d: %#v", len(transport.calls), transport.calls)
	}
}

func TestAnimeDetailAndCatalogueQueriesUseTheAnimeSection(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v4/movies/anime|zh-TW": []byte(`{"success":1,"data":{"movie":{"id":"anime","number":"GLOD-0436","type":4}}}`),
		"/api/v2/tags|zh-TW":         fixtureFile(t, "tags_zh.json"),
	}}
	client := clientWithTransport(transport)
	detail, err := client.MovieDetail(t.Context(), "anime")
	if err != nil || detail.Zone != domain.ZoneAnime || detail.Code != "GLOD-0436" {
		t.Fatalf("anime detail = %#v, error = %v", detail, err)
	}
	params, err := buildSearchParams("GLOD-0436", domain.SearchOptions{Zone: domain.ZoneAnime})
	if err != nil || params.Get("movie_type") != "4" {
		t.Fatalf("anime search parameters = %#v, error = %v", params, err)
	}
	params, err = buildBrowseParams(domain.BrowseOptions{Zone: domain.ZoneAnime, Page: 1, Limit: 20, Sort: "release", Order: "desc"})
	if err != nil || params.Get("filter_by") != "4:t:::::" {
		t.Fatalf("anime browse parameters = %#v, error = %v", params, err)
	}
	if _, err := client.Tags(t.Context(), domain.ZoneAnime); err != nil {
		t.Fatal(err)
	}
	if call := transport.calls[len(transport.calls)-1]; call.params.Get("type") != "4" {
		t.Fatalf("anime taxonomy used another section: %#v", call)
	}
}

func TestMoviePreviewsOmitEmptyEntriesAndKeepAvailableURLs(t *testing.T) {
	movie, err := movieFromWire(t.Context(), wireMovie{ID: "preview-movie", Number: "090826_100", PreviewImages: []wirePreviewImage{
		{},
		{ThumbURL: "https://media.example/thumb.jpg", LargeURL: "https://media.example/image.jpg"},
		{},
		{ThumbURL: "https://media.example/thumbnail-only.jpg"},
		{LargeURL: "https://media.example/original-only.jpg"},
	}})
	if err != nil || len(movie.PreviewImages) != 3 {
		t.Fatalf("previews = %#v, error = %v", movie.PreviewImages, err)
	}
	if movie.PreviewImages[0].Original != "https://media.example/image.jpg" ||
		movie.PreviewImages[1].Thumbnail != "https://media.example/thumbnail-only.jpg" ||
		movie.PreviewImages[2].Original != "https://media.example/original-only.jpg" {
		t.Fatalf("available preview URLs changed: %#v", movie.PreviewImages)
	}
}

func TestResolveMovieIDKeepsLetterVariantsDistinct(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v2/search|zh-TW": []byte(`{"success":1,"data":{"movies":[{"id":"base","number":"FJIN-106"},{"id":"variant","number":"FJIN-106a"}]}}`),
	}}
	client := clientWithTransport(transport)
	for code, want := range map[string]string{"FJIN-106": "base", "fjin106a": "variant"} {
		id, err := client.ResolveMovieID(t.Context(), code)
		if err != nil {
			t.Fatal(err)
		}
		if id != want {
			t.Errorf("ResolveMovieID(%q) = %q, want %q", code, id, want)
		}
	}
}

func TestResolveMovieIDMatchesDistributorLabelInSameResults(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v2/search|zh-TW": []byte(`{"success":1,"data":{"movies":[
			{"id":"similar","number":"LUXU-1099"},
			{"id":"longer","number":"LUXU-18990"},
			{"id":"variant","number":"LUXU-1899-C"},
			{"id":"exact","number":"LUXU-1899"}
		]}}`),
	}}
	client := clientWithTransport(transport)
	id, err := client.ResolveMovieID(t.Context(), "259luxu1899")
	if err != nil || id != "exact" {
		t.Fatalf("259luxu1899 resolved to %q, error = %v", id, err)
	}
	if len(transport.calls) != 1 || transport.calls[0].params.Get("q") != "259LUXU-1899" {
		t.Fatalf("search did not use the filename number once: %#v", transport.calls)
	}
}

func TestResolveMovieIDPrefersExactOverRelaxedCandidate(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v2/search|zh-TW": []byte(`{"success":1,"data":{"movies":[
			{"id":"relaxed","number":"GIRI-001"},
			{"id":"exact","number":"1000GIRI-001"}
		]}}`),
	}}
	client := clientWithTransport(transport)
	if id, err := client.ResolveMovieID(t.Context(), "1000giri001"); err != nil || id != "exact" {
		t.Fatalf("1000giri001 resolved to %q, error = %v", id, err)
	}
}

func TestResolveMovieIDSearchesRelaxedCandidates(t *testing.T) {
	empty := []byte(`{"success":1,"data":{"movies":[]}}`)
	transport := &queryFixtureTransport{responses: map[string][]byte{
		"326IHD-005":       empty,
		"IHD-005":          empty,
		"326IHD-5":         empty,
		"IHD-5":            []byte(`{"success":1,"data":{"movies":[{"id":"ihd-5","number":"IHD-5"}]}}`),
		"200GANA-3458":     empty,
		"GANA-3458":        []byte(`{"success":1,"data":{"movies":[{"id":"gana-3458","number":"GANA-3458"}]}}`),
		"CARIB-060326-001": empty,
		"060326-001":       []byte(`{"success":1,"data":{"movies":[{"id":"carib","number":"060326-001"}]}}`),
	}}
	client := clientWithTransport(transport)
	for code, want := range map[string]string{
		"326IHD-005":       "ihd-5",
		"200GANA-3458":     "gana-3458",
		"CARIB-060326-001": "carib",
	} {
		if id, err := client.ResolveMovieID(t.Context(), code); err != nil || id != want {
			t.Errorf("ResolveMovieID(%q) = %q, error = %v; want %q", code, id, err, want)
		}
	}
}

func TestResolveMovieIDKeepsMultipartNumbersDistinct(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v2/search|zh-TW": []byte(`{"success":1,"data":{"movies":[
			{"id":"partial","number":"HEYDOUGA-4030"},
			{"id":"different-series","number":"HEYDOUGA-4031-2347"},
			{"id":"different-movie","number":"HEYDOUGA-4030-2348"},
			{"id":"exact","number":"heydouga-4030-2347"}
		]}}`),
	}}
	client := clientWithTransport(transport)
	for code, want := range map[string]string{
		"heydouga4030_2347":  "exact",
		"HEYDOUGA-4030-2348": "different-movie",
		"HEYDOUGA-4031-2347": "different-series",
	} {
		id, err := client.ResolveMovieID(t.Context(), code)
		if err != nil {
			t.Fatal(err)
		}
		if id != want {
			t.Errorf("ResolveMovieID(%q) = %q, want %q", code, id, want)
		}
	}
}

func TestResolveMovieIDKeepsNamedNumbersDistinct(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v2/search|zh-TW": fixtureFile(t, "browse_catalogue_numbers.json"),
	}}
	client := clientWithTransport(transport)
	for code, want := range map[string]string{
		"scute1575_itsuki":        "named-itsuki",
		"SCUTE-1575-NANAMI":       "named-nanami",
		"SCUTE-15750-ITSUKI":      "named-other-number",
		"EXAMPLE-123-model2-0001": "named-variant",
	} {
		id, err := client.ResolveMovieID(t.Context(), code)
		if err != nil {
			t.Fatal(err)
		}
		if id != want {
			t.Errorf("ResolveMovieID(%q) = %q, want %q", code, id, want)
		}
	}
	for _, code := range []string{"SCUTE-1575", "SCUTE-1575-OTHER", "EXAMPLE-123-model2-0002"} {
		if id, err := client.ResolveMovieID(t.Context(), code); err == nil {
			t.Errorf("ResolveMovieID(%q) accepted a partial match: %q", code, id)
		}
	}
}

func TestTagsOnlyRequestsTraditionalChinese(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v2/tags|zh-TW": fixtureFile(t, "tags_zh.json"),
	}}
	client := clientWithTransport(transport)

	categories, err := client.Tags(t.Context(), domain.ZoneCensored)
	if err != nil {
		t.Fatal(err)
	}
	if len(categories) != 1 || categories[0].ID != "category-1" || categories[0].Name != "身材" {
		t.Fatalf("categories = %#v", categories)
	}
	if len(categories[0].Tags) != 2 || categories[0].Tags[0].Name != "高挑" ||
		categories[0].Tags[0].ID != "tag-1" {
		t.Fatalf("tags = %#v", categories[0].Tags)
	}
	if len(transport.calls) != 1 || transport.calls[0].language != defaultLanguage {
		t.Fatalf("calls = %#v", transport.calls)
	}
}

func TestQueryOptionsRejectUnsupportedZones(t *testing.T) {
	for _, zone := range []domain.Zone{"invalid", domain.ZoneUnknown} {
		if _, err := buildSearchParams("ABP-123", domain.SearchOptions{Zone: zone}); err == nil {
			t.Fatalf("search accepted unsupported zone %q", zone)
		}
		if _, err := buildBrowseParams(domain.BrowseOptions{Zone: zone, Page: 1, Limit: 20, Sort: "release", Order: "desc"}); err == nil {
			t.Fatalf("browse accepted unsupported zone %q", zone)
		}
		if _, err := (&Client{}).Tags(t.Context(), zone); err == nil {
			t.Fatalf("tags accepted unsupported zone %q", zone)
		}
	}
	if _, err := buildBrowseParams(domain.BrowseOptions{Zone: domain.ZoneAll, Page: 1, Limit: 20, Sort: "release", Order: "desc"}); err == nil {
		t.Fatal("browse accepted the all zone")
	}
}
