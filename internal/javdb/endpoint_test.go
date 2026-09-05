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
	if len(movie.Actors) != 1 || movie.Actors[0].NameZHT != "演員" {
		t.Fatalf("actors = %#v", movie.Actors)
	}
	if len(movie.Tags) != 1 || movie.Tags[0].CategoryID != "category-1" {
		t.Fatalf("tags = %#v", movie.Tags)
	}
	if movie.Series == nil || movie.Maker == nil || movie.Director == nil {
		t.Fatalf("graph = %#v %#v %#v", movie.Series, movie.Maker, movie.Director)
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

func TestTagsMergesEnglishAndTraditionalChineseByID(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v2/tags|en":    fixtureFile(t, "tags_en.json"),
		"/api/v2/tags|zh-TW": fixtureFile(t, "tags_zh.json"),
	}}
	client := clientWithTransport(transport)

	categories, err := client.Tags(t.Context(), ZoneCensored)
	if err != nil {
		t.Fatal(err)
	}
	if len(categories) != 1 || categories[0].Name != "Body" || categories[0].NameZHT != "身材" {
		t.Fatalf("categories = %#v", categories)
	}
	if len(categories[0].Tags) != 2 || categories[0].Tags[0].NameZHT != "高挑" ||
		categories[0].Tags[0].CategoryID != "category-1" {
		t.Fatalf("tags = %#v", categories[0].Tags)
	}
	if len(transport.calls) != 2 || transport.calls[0].language != "en" ||
		transport.calls[1].language != defaultLanguage {
		t.Fatalf("calls = %#v", transport.calls)
	}
}

func TestQueryOptionsRejectUnsupportedZones(t *testing.T) {
	if _, err := buildSearchParams("ABP-123", SearchOptions{Zone: "invalid"}); err == nil {
		t.Fatal("search accepted an unsupported zone")
	}
	if _, err := buildBrowseParams(BrowseOptions{Zone: ZoneAll}); err == nil {
		t.Fatal("browse accepted the all zone")
	}
}
