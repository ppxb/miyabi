package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/catalogue"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/time/rate"
)

const pacopacomamaDetail = `{
  "MovieID":"010126_100","SiteID":2469,"Status":true,"Title":"Official title",
  "Release":"2026-01-01","Duration":3703,"AvgRating":4.5,
  "ActorID":[12,12,99],"ActressesList":{"12":{"NameJa":"Actor"},"88":{"NameJa":"Unrelated"}},
  "UC":[6,25,6],"UcNameList":{"6":{"NameJa":"Category"},"25":{"NameEn":"Second category"}},
  "SeriesID":627,"Series":"Series","Desc":"Excluded synopsis",
  "ThumbUltra":"https://www.pacopacomama.com/moviepages/010126_100/images/l_hd.jpg",
  "ThumbHigh":"https://www.pacopacomama.com/moviepages/010126_100/images/l_hd.jpg",
  "MovieThumb":"/assets/sample/010126_100/l_thum.jpg","Gallery":true
}`

const pacopacomamaGallery = `{"MovieID":"010126_100","Rows":[
  {"Img":"movie_gallery/sample/010126_100/first.jpg","Protected":false},
  {"Img":"movie_gallery/sample/010126_100/second.jpg","Protected":false},
  {"Img":"movie_gallery/sample/010126_100/first.jpg","Protected":false},
  {"Img":"movie_gallery/member/010126_100/private.jpg","Protected":true},
  {"Img":"movie_gallery/sample/010126_100/protected.jpg","Protected":true},
  {"Img":"movie_gallery/member/010126_100/private.jpg","Protected":false},
  {"Img":"movie_gallery/sample/010126_100/no-permission.jpg"},
  {"Img":"movie_gallery/sample/010226_100/other.jpg","Protected":false},
  {"Img":"movie_gallery/sample/010126_100/../../other.jpg","Protected":false},
  {"Img":"movie_gallery/sample/010126_100/first__@120.jpg","Protected":false},
  {"Img":"https://foreign.example/image.jpg","Protected":false}
]}`

func testPacopacomama(t *testing.T, handler http.HandlerFunc) *Pacopacomama {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	s := NewPacopacomama(nil)
	s.base, s.http, s.limiter = server.URL+"/", server.Client(), rate.NewLimiter(rate.Inf, 1)
	t.Cleanup(s.Close)
	return s
}

func pacopacomamaFixtureHandler(t *testing.T, calls *atomic.Int32) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.Header.Get("User-Agent") == "" || r.Header.Get("Referer") == "" {
			t.Error("incorrect source request")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/dyn/phpauto/movie_details/movie_id/010126_100.json":
			_, _ = w.Write([]byte(pacopacomamaDetail))
		case "/dyn/dla/json/movie_gallery/010126_100.json":
			_, _ = w.Write([]byte(pacopacomamaGallery))
		default:
			t.Errorf("unexpected source path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func TestPacopacomamaFetchesConfirmedMetadataAndPublicOriginals(t *testing.T) {
	var calls atomic.Int32
	s := testPacopacomama(t, pacopacomamaFixtureHandler(t, &calls))
	m, err := s.Fetch(t.Context(), domain.MovieRef{Code: "010126-100"})
	if err != nil {
		t.Fatal(err)
	}
	d := m.Detail
	if d.ID != "" || d.Code != "010126_100" || d.Title != "Official title" || d.OriginTitle != d.Title || d.Zone != domain.ZoneUncensored ||
		!reflect.DeepEqual(d.Sources, []domain.SourceID{{Provider: "pacopacomama", ID: "010126_100"}}) {
		t.Fatalf("wrong identity: %+v", d)
	}
	if d.ReleaseDate != "2026-01-01" || d.Duration != 61 || d.Rating != 4.5 || d.RatingMax != 5 || d.RatingSource != "pacopacomama" {
		t.Fatalf("wrong metadata: %+v", d)
	}
	if len(d.Actors) != 1 || d.Actors[0].ID != "12" || d.Actors[0].Name != "Actor" || d.Actors[0].Provider != "pacopacomama" ||
		d.Series == nil || d.Series.ID != "627" || d.Maker == nil || d.Maker.Name != "Pacopacomama" {
		t.Fatalf("wrong entities: %+v", d)
	}
	wantTags := []domain.Tag{{Provider: "pacopacomama", ID: "6", Name: "Category"}, {Provider: "pacopacomama", ID: "25", Name: "Second category"}}
	wantImages := []domain.ImageCandidate{
		{Provider: "pacopacomama", Role: "cover", URL: pacopacomamaBase + "moviepages/010126_100/images/l_hd.jpg", Layout: domain.CoverSingle},
		{Provider: "pacopacomama", Role: "cover", URL: pacopacomamaBase + "assets/sample/010126_100/l_thum.jpg", Layout: domain.CoverSingle},
		{Provider: "pacopacomama", Role: "preview", URL: pacopacomamaBase + "dyn/dla/images/movie_gallery/sample/010126_100/first.jpg"},
		{Provider: "pacopacomama", Role: "preview", URL: pacopacomamaBase + "dyn/dla/images/movie_gallery/sample/010126_100/second.jpg"},
	}
	if !reflect.DeepEqual(d.Tags, wantTags) || !reflect.DeepEqual(m.Images, wantImages) || d.Cover != wantImages[0].URL ||
		len(d.PreviewImages) != 2 || !d.HasPreview || d.PreviewImages[0].Original != wantImages[2].URL || calls.Load() != 2 {
		t.Fatalf("wrong artwork, tags or requests: %+v calls=%d", m, calls.Load())
	}
	encoded, err := json.Marshal(m)
	if err != nil || bytes.Contains(encoded, []byte("Excluded synopsis")) {
		t.Fatal("unexpected synopsis or serialization error")
	}
}

func TestPacopacomamaRejectsUnsupportedCodesAndUnconfirmedResponses(t *testing.T) {
	s := testPacopacomama(t, func(http.ResponseWriter, *http.Request) { t.Error("unsupported code fetched") })
	if s.ID() != "pacopacomama" || !s.Supports("010126_100") || !s.Supports("010126-100") {
		t.Fatal("numeric catalogue routing failed")
	}
	for _, code := range []string{"ABP-123", "HEYZO-1234", "010126_100A", "../010126_100", "010126_100-2", "PACOPACOMAMA-010126-100"} {
		if s.Supports(code) {
			t.Fatalf("unsupported code %s", code)
		}
		if _, err := s.Fetch(t.Context(), domain.MovieRef{Code: code}); !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("invalid code %s: %v", code, err)
		}
	}
	for _, tt := range []struct {
		name, body string
		status     int
		miss       bool
	}{
		{"404", "", 404, true},
		{"inactive", strings.Replace(pacopacomamaDetail, `"Status":true`, `"Status":false`, 1), 200, true},
		{"wrong movie", strings.Replace(pacopacomamaDetail, `"MovieID":"010126_100"`, `"MovieID":"010226_100"`, 1), 200, false},
		{"wrong site", strings.Replace(pacopacomamaDetail, `"SiteID":2469`, `"SiteID":1`, 1), 200, false},
		{"missing title", strings.Replace(pacopacomamaDetail, "Official title", "", 1), 200, false},
		{"HTML response", "<html>Unavailable</html>", 200, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			s := testPacopacomama(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			_, err := s.Fetch(t.Context(), domain.MovieRef{Code: "010126_100"})
			if err == nil || errors.Is(err, metadata.ErrNotFound) != tt.miss || calls != 1 {
				t.Fatalf("invalid response accepted or gallery fetched: %v calls=%d", err, calls)
			}
		})
	}
}

func TestPacopacomamaCoverOwnershipAndOptionalFields(t *testing.T) {
	for _, ref := range []string{
		"https://foreign.example/moviepages/010126_100/images/l_hd.jpg",
		"https://www.pacopacomama.com/moviepages/010226_100/images/l_hd.jpg",
		"https://www.pacopacomama.com/assets/member/010126_100/l.jpg",
		"https://user@www.pacopacomama.com/assets/sample/010126_100/l.jpg",
		"http://www.pacopacomama.com/assets/sample/010126_100/l.jpg",
		"https://www.pacopacomama.com:8080/assets/sample/010126_100/l.jpg",
	} {
		m := pacopacomamaMetadata(pacopacomamaMovie{MovieID: "010126_100", TitleEn: "Title", ThumbHigh: ref})
		if m.Detail.Title != "Title" || m.Detail.Cover != "" || len(m.Images) != 0 {
			t.Fatalf("invalid cover accepted: %s", ref)
		}
	}
}

func TestPacopacomamaGalleryAvailabilityAndFailures(t *testing.T) {
	for _, tt := range []struct {
		name, detail, gallery, path string
		status, requests, previews  int
		wantError, retry            bool
	}{
		{"no gallery", strings.Replace(pacopacomamaDetail, `"Gallery":true`, `"Gallery":false`, 1), "", "", 200, 1, 0, false, false},
		{"gallery removed", pacopacomamaDetail, "", "/dyn/dla/json/movie_gallery/010126_100.json", 404, 2, 0, false, false},
		{"gallery unavailable", pacopacomamaDetail, "", "/dyn/dla/json/movie_gallery/010126_100.json", 503, 2, 0, true, true},
		{"wrong gallery", pacopacomamaDetail, `{"MovieID":"010226_100","Rows":[]}`, "/dyn/dla/json/movie_gallery/010126_100.json", 200, 2, 0, true, false},
		{"invalid gallery", pacopacomamaDetail, `<html>Unavailable</html>`, "/dyn/dla/json/movie_gallery/010126_100.json", 200, 2, 0, true, false},
		{"filename gallery", strings.Replace(pacopacomamaDetail, `"Gallery":true`, `"HasGallery":true`, 1), `{"MovieID":"010126_100","Rows":[{"Filename":"g_b1.jpg","Protected":false},{"Filename":"g_b2.jpg","Protected":true}]}`, "/dyn/phpauto/movie_galleries/movie_id/010126_100.json", 200, 2, 1, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			s := testPacopacomama(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if strings.Contains(r.URL.Path, "movie_details") {
					_, _ = w.Write([]byte(tt.detail))
					return
				}
				if r.URL.Path != tt.path {
					t.Errorf("wrong gallery endpoint: %s", r.URL.Path)
				}
				w.Header().Set("Retry-After", "120")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.gallery))
			})
			m, err := s.Fetch(t.Context(), domain.MovieRef{Code: "010126_100"})
			if (err != nil) != tt.wantError || calls != tt.requests || len(m.Detail.PreviewImages) != tt.previews {
				t.Fatalf("gallery result: %+v %v calls=%d", m, err, calls)
			}
			if !tt.wantError && m.Detail.Cover == "" {
				t.Fatal("usable cover lost without gallery")
			}
			if tt.retry {
				_, err = s.Fetch(t.Context(), domain.MovieRef{Code: "010226_100"})
				if delay, retry := domain.RetryDelay(err); !retry || delay < 119*time.Second || calls != tt.requests {
					t.Fatalf("gallery failure bypassed shared cooldown: %v", err)
				}
			}
			if tt.previews > 0 && m.Detail.PreviewImages[0].Original != pacopacomamaBase+"assets/sample/010126_100/l/g_b1.jpg" {
				t.Fatal("incorrect public gallery original")
			}
		})
	}
}

func TestPacopacomamaResolveKeepsJavDBEntitiesAndWorksWithoutCatalogue(t *testing.T) {
	for _, indexed := range []bool{true, false} {
		t.Run(map[bool]string{true: "indexed", false: "official only"}[indexed], func(t *testing.T) {
			var calls atomic.Int32
			source := testPacopacomama(t, pacopacomamaFixtureHandler(t, &calls))
			catalogueSource := &javdbStub{}
			if indexed {
				catalogueSource.search = []catalogue.Movie{{Movie: domain.Movie{Code: "010126_100", ID: "javdb-id"}}}
				catalogueSource.detail = domain.MovieDetail{Movie: domain.Movie{
					Code: "010126_100", ID: "javdb-id", Title: "Catalogue title",
					Actors: []domain.Actor{{ID: "catalogue-actor", Name: "Actor"}},
					Tags:   []domain.Tag{{ID: "catalogue-tag", Name: "Category", CategoryID: "catalogue-category"}},
				}}
			}
			store, err := database.Open(t.Context(), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			s, err := metadata.New(t.Context(), store.Client, source, NewJavDB(catalogueSource))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if !reflect.DeepEqual(s.Settings(), []metadata.SourceSetting{{ID: "pacopacomama", Enabled: true}}) {
				t.Fatalf("source not enabled by default: %+v", s.Settings())
			}
			for range 2 {
				m, err := s.Resolve(t.Context(), domain.MovieRef{Code: "PACOPACOMAMA-010126-100"})
				if err != nil || len(m.Detail.PreviewImages) != 2 || m.Images[0].Provider != "pacopacomama" {
					t.Fatalf("source not integrated: %+v %v", m, err)
				}
				if indexed {
					if m.Detail.ID != "javdb-id" || m.Detail.Title != "Catalogue title" || m.Detail.Actors[0].ID != "catalogue-actor" || m.Detail.Tags[0].ID != "catalogue-tag" || m.Detail.Tags[0].CategoryID != "catalogue-category" || m.Detail.FieldSources["tags"] != "javdb" {
						t.Fatalf("catalogue identity replaced: %+v", m.Detail)
					}
				} else if m.Detail.ID != "" || m.Detail.Title != "Official title" || m.Detail.FieldSources["title"] != "pacopacomama" {
					t.Fatalf("official-only metadata lost: %+v", m.Detail)
				}
			}
			if calls.Load() != 2 {
				t.Fatalf("source cache bypassed: %d requests", calls.Load())
			}
		})
	}
}

func TestPacopacomamaLive(t *testing.T) {
	codes := os.Getenv("MIYABI_TEST_PACOPACOMAMA_CODES")
	if codes == "" {
		t.Skip("set MIYABI_TEST_PACOPACOMAMA_CODES for live verification; images are decoded in memory")
	}
	proxy, err := netx.NewProxyManager(netx.ProxyConfig{Enabled: os.Getenv("MIYABI_TEST_PROXY") != "", URL: os.Getenv("MIYABI_TEST_PROXY")})
	if err != nil {
		t.Fatal(err)
	}
	s := NewPacopacomama(proxy)
	defer s.Close()
	for _, code := range strings.Split(codes, ",") {
		t.Run(code, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
			defer cancel()
			m, err := s.Fetch(ctx, domain.MovieRef{Code: code})
			if err != nil {
				t.Fatal(err)
			}
			if !codeid.IsFormatEquivalent(m.Detail.Code, code) || m.Detail.ID != "" || m.Detail.Title == "" || m.Detail.Duration == 0 || m.Detail.ReleaseDate == "" || len(m.Images) == 0 {
				t.Fatal("missing identity or core metadata")
			}
			t.Logf("%s: %d public previews", code, len(m.Detail.PreviewImages))
			for _, candidate := range m.Images {
				media, err := s.Media(ctx, candidate.URL)
				if err != nil {
					t.Fatal(err)
				}
				decoded, _, err := image.Decode(bytes.NewReader(media.Body))
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("%s %s: %dx%d", candidate.Role, candidate.URL, decoded.Bounds().Dx(), decoded.Bounds().Dy())
			}
		})
	}
}
