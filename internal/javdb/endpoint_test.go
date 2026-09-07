package javdb

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

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

	movies, err := client.Search(t.Context(), " ABP-123 ", SearchOptions{
		Zone:     ZoneCensored,
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

	movies, err := client.Browse(t.Context(), BrowseOptions{
		Zone:   ZoneCensored,
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
	if movie.Zone != ZoneCensored || len(movie.ActorMovies) != 1 || len(movie.RelatedMovies) != 1 {
		t.Fatalf("zone = %s, actor movies = %#v, related movies = %#v", movie.Zone, movie.ActorMovies, movie.RelatedMovies)
	}
	if movie.ActorMovies[0].Code != "ABP-124" || movie.RelatedMovies[0].Code != "SONE-001A" || movie.RelatedMovies[0].Thumbnail != "https://media.example/related-movie.jpg" {
		t.Fatalf("recommendations = %#v, %#v", movie.ActorMovies, movie.RelatedMovies)
	}
}

func TestBrowseBuildsEntityFilters(t *testing.T) {
	for _, test := range []struct {
		kind EntityType
		want string
	}{
		{EntityActor, ":a:entity-1"},
		{EntitySeries, ":s:entity-1"},
		{EntityMaker, ":m:entity-1"},
		{EntityDirector, ":d:entity-1"},
	} {
		t.Run(string(test.kind), func(t *testing.T) {
			options := BrowseOptions{EntityType: test.kind, EntityID: "entity-1", Sort: "release", Order: "desc", Page: 2, Limit: 20}
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
			options.Zone = ZoneFC2
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

func TestTagsOnlyRequestsTraditionalChinese(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v2/tags|zh-TW": fixtureFile(t, "tags_zh.json"),
	}}
	client := clientWithTransport(transport)

	categories, err := client.Tags(t.Context(), ZoneCensored)
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
	if _, err := buildSearchParams("ABP-123", SearchOptions{Zone: "invalid"}); err == nil {
		t.Fatal("search accepted an unsupported zone")
	}
	if _, err := buildBrowseParams(BrowseOptions{Zone: ZoneAll, Page: 1, Limit: 20, Sort: "release", Order: "desc"}); err == nil {
		t.Fatal("browse accepted the all zone")
	}
}
