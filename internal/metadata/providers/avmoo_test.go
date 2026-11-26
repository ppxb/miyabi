package providers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"golang.org/x/time/rate"
)

const avmooSearchFixture = `{"code":200,"data":[{"movieId":"fixture-movie","movieFanHao":"SSIS-001","title_ja":"Fixture Title","releaseDate":"2024-01-02","length":120}]}`

const avmooDetailFixture = `{"code":200,"data":{
	"movieId":"fixture-movie","movieFanHao":"SSIS-001",
	"title":"Fixture Title","title_ja":"Fixture Title JA","title_en":"Fixture Title EN",
	"releaseDate":"2024-01-02","length":120,
	"posterLarge":"https://jp.netcdn.space/cover-large.jpg",
	"posterSmall":"https://jp.netcdn.space/cover-small.jpg",
	"sampleLarge":["https://jp.netcdn.space/sample-1.jpg","https://jp.netcdn.space/sample-2.jpg"],
	"sampleSmall":["https://file.netcdn.space/sample-1.jpg","https://file.netcdn.space/sample-2.jpg"],
	"star":[
		{"starId":"star-1","starName_ja":"Fixture Actress","starName_tw":"固定女優","avatar":"fixture-1.jpg","avatarUrl":"https://file.netcdn.space/avatar/fixture-1.jpg"},
		{"starId":"star-2","starName_ja":"Second Actress"}
	],
	"genre":[{"genreId":"genre-1","genreName_ja":"Fixture Genre","genreName_tw":"分類"}],
	"label":{"labelId":"label-1","labelName_ja":"Fixture Label"},
	"series":{"seriesId":"series-1","seriesName_ja":"Fixture Series"},
	"studio":{"studioId":"studio-1","studioName_ja":"Fixture Studio"},
	"director":{"directorId":"director-1","directorName_ja":"Fixture Director"}
}}`

func newAvmooFixture(t *testing.T, searchBody, detailBody string) (*Avmoo, *int) {
	t.Helper()
	detailCalls := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" ||
			!strings.Contains(request.Header.Get("User-Agent"), "Mozilla/5.0") || request.Header.Get("Referer") != server.URL+"/" {
			t.Errorf("request method=%s content-type=%q user-agent=%q referer=%q",
				request.Method, request.Header.Get("Content-Type"), request.Header.Get("User-Agent"), request.Header.Get("Referer"))
		}
		switch request.URL.Path {
		case "/jav/data/api/search":
			var args []json.RawMessage
			if err := json.NewDecoder(request.Body).Decode(&args); err != nil || len(args) != 3 {
				t.Errorf("search body=%v err=%v", args, err)
			}
			var query map[string]string
			if len(args) == 3 {
				if err := json.Unmarshal(args[0], &query); err != nil || query["search"] != "SSIS-001" || query["lang"] != "ja" {
					t.Errorf("search query=%v err=%v", query, err)
				}
			}
			_, _ = io.WriteString(w, searchBody)
		case "/jav/data/api/getMovie":
			detailCalls++
			var args []string
			if err := json.NewDecoder(request.Body).Decode(&args); err != nil || len(args) != 2 || args[0] != "fixture-movie" || args[1] != "ja" {
				t.Errorf("detail body=%v err=%v", args, err)
			}
			_, _ = io.WriteString(w, detailBody)
		default:
			http.NotFound(w, request)
		}
	}))
	t.Cleanup(server.Close)
	provider := &Avmoo{client: client{http: server.Client(), limiter: rate.NewLimiter(rate.Inf, 1), base: server.URL + "/"}}
	return provider, &detailCalls
}

func TestAvmooFetchMapsMetadataAndActors(t *testing.T) {
	provider, detailCalls := newAvmooFixture(t, avmooSearchFixture, avmooDetailFixture)
	result, err := provider.Fetch(t.Context(), domain.MovieRef{Code: "ssis-001"})
	if err != nil {
		t.Fatal(err)
	}
	m := result.Detail
	if m.Code != "SSIS-001" || m.Title != "Fixture Title JA" || m.OriginTitle != "Fixture Title JA" ||
		m.ReleaseDate != "2024-01-02" || m.Duration != 120 {
		t.Fatalf("movie=%+v", m.Movie)
	}
	if len(m.Sources) != 1 || m.Sources[0] != (domain.SourceID{Provider: "avmoo", ID: "fixture-movie"}) {
		t.Fatalf("sources=%+v", m.Sources)
	}
	if len(m.Actors) != 2 {
		t.Fatalf("actors=%+v", m.Actors)
	}
	if m.Actors[0].Provider != "avmoo" || m.Actors[0].ID != "star-1" || m.Actors[0].Name != "Fixture Actress" ||
		m.Actors[0].NameZHT != "固定女優" || m.Actors[0].Avatar != "https://file.netcdn.space/avatar/fixture-1.jpg" {
		t.Fatalf("actor=%+v", m.Actors[0])
	}
	if m.Actors[1].Avatar != "" {
		t.Fatalf("actor without avatar=%+v", m.Actors[1])
	}
	if len(m.Tags) != 1 || m.Tags[0].ID != "genre-1" || m.Tags[0].Name != "Fixture Genre" || m.Tags[0].NameZHT != "分類" {
		t.Fatalf("tags=%+v", m.Tags)
	}
	if m.Maker == nil || m.Maker.ID != "studio-1" || m.Maker.Name != "Fixture Studio" {
		t.Fatalf("maker=%+v", m.Maker)
	}
	if m.Series == nil || m.Series.ID != "series-1" || m.Series.Name != "Fixture Series" {
		t.Fatalf("series=%+v", m.Series)
	}
	if m.Director == nil || m.Director.ID != "director-1" || m.Director.Name != "Fixture Director" {
		t.Fatalf("director=%+v", m.Director)
	}
	if m.Cover != "https://jp.netcdn.space/cover-large.jpg" || m.Thumbnail != "https://jp.netcdn.space/cover-small.jpg" {
		t.Fatalf("cover=%q thumbnail=%q", m.Cover, m.Thumbnail)
	}
	if len(m.PreviewImages) != 2 || m.PreviewImages[0].Thumbnail != "https://file.netcdn.space/sample-1.jpg" || !m.HasPreview {
		t.Fatalf("previews=%+v", m.PreviewImages)
	}
	if len(result.Images) != 4 || result.Images[0].Role != "cover" || result.Images[2].Role != "preview" {
		t.Fatalf("images=%+v", result.Images)
	}
	if *detailCalls != 1 {
		t.Fatalf("detail calls=%d", *detailCalls)
	}
}

func TestAvmooRejectsSimilarSearchHit(t *testing.T) {
	provider, detailCalls := newAvmooFixture(t,
		`{"code":200,"data":[{"movieId":"fixture-movie","movieFanHao":"SSIS-002"}]}`,
		avmooDetailFixture)
	if _, err := provider.Fetch(t.Context(), domain.MovieRef{Code: "SSIS-001"}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if *detailCalls != 0 {
		t.Fatalf("detail fetched for a non-matching hit: %d", *detailCalls)
	}
}

func TestAvmooReturnsNotFoundForEmptySearch(t *testing.T) {
	provider, _ := newAvmooFixture(t, `{"code":200,"data":[]}`, avmooDetailFixture)
	if _, err := provider.Fetch(t.Context(), domain.MovieRef{Code: "SSIS-001"}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestAvmooReportsIncompleteDetail(t *testing.T) {
	provider, _ := newAvmooFixture(t, avmooSearchFixture, `{"code":400,"message":"missing parameters","data":null}`)
	_, err := provider.Fetch(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if err == nil || errors.Is(err, metadata.ErrNotFound) || !domain.IsKind(err, domain.KindUpstream) {
		t.Fatalf("want upstream error, got %v", err)
	}
}

func TestAvmooSupportsExcludesOfficialFamilies(t *testing.T) {
	provider := &Avmoo{}
	for code, want := range map[string]bool{
		"SSIS-001":      true,
		"259LUXU-1899":  true,
		"FC2-PPV-12345": false,
		"fc2-12345":     false,
		"HEYZO-1234":    false,
		"":              false,
	} {
		if got := provider.Supports(code); got != want {
			t.Errorf("Supports(%q)=%v want %v", code, got, want)
		}
	}
}

func TestAvmooSearchEndpointFailureIsUpstream(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	provider := &Avmoo{client: client{http: server.Client(), limiter: rate.NewLimiter(rate.Inf, 1), base: server.URL + "/"}}
	_, err := provider.Fetch(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if !domain.IsKind(err, domain.KindUpstream) || errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("want a pure upstream error, got %v", err)
	}
}

func TestAvmooDetailEndpointFailureIsUpstream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/jav/data/api/search":
			_, _ = io.WriteString(w, avmooSearchFixture)
		default:
			http.NotFound(w, request)
		}
	}))
	t.Cleanup(server.Close)
	provider := &Avmoo{client: client{http: server.Client(), limiter: rate.NewLimiter(rate.Inf, 1), base: server.URL + "/"}}
	_, err := provider.Fetch(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if !domain.IsKind(err, domain.KindUpstream) || errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("want a pure upstream error, got %v", err)
	}
}

func TestAvmooSearchEnvelopeErrorIsUpstream(t *testing.T) {
	provider, _ := newAvmooFixture(t, `{"code":500,"message":"boom","data":[]}`, avmooDetailFixture)
	_, err := provider.Fetch(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if !domain.IsKind(err, domain.KindUpstream) {
		t.Fatalf("want upstream error, got %v", err)
	}
}

func TestAvmooMissingMovieIsNotFound(t *testing.T) {
	provider, _ := newAvmooFixture(t, avmooSearchFixture, `{"code":404,"message":"missing","data":null}`)
	if _, err := provider.Fetch(t.Context(), domain.MovieRef{Code: "SSIS-001"}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestAvmooMetadataKeepsLocalizedTitleOutOfOriginTitle(t *testing.T) {
	result := avmooMetadata("ABC-001", avmooMovie{Title: "Fixture Fallback", TitleCN: "中文标题"})
	if result.Detail.Title != "Fixture Fallback" || result.Detail.OriginTitle != "" {
		t.Fatalf("title=%q origin=%q", result.Detail.Title, result.Detail.OriginTitle)
	}
}

func TestAvmooMediaRejectsForeignHosts(t *testing.T) {
	provider := &Avmoo{client: client{hosts: []string{"avmoo.shop", "netcdn.space"}}}
	if _, err := provider.Media(t.Context(), "https://evil.example/avatar.jpg"); !domain.IsKind(err, domain.KindInvalid) {
		t.Fatalf("want invalid media host error, got %v", err)
	}
}
