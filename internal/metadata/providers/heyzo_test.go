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
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/time/rate"
)

const heyzoPage = `<html><head>
<meta property="og:url" content="//www.heyzo.com/moviepages/1234/index.html">
<meta property="og:image" content="//www.heyzo.com/contents/3000/1234/images/player_thumbnail.jpg">
</head><body><div id="movie"><h1>Official &amp; title</h1>
<script type="application/ld+json">{"@type":"Movie","duration":"PT1H1M31S","dateCreated":"2026-03-14","description":"Excluded synopsis"}</script>
<table class="movieInfo">
<tr class="table-release-day"><td>Release</td><td>2026-03-15</td></tr>
<tr class="table-actor"><td><a href="/listpages/actor_580_1.html?sort=pop">Actor</a><a href="/listpages/actor_580_1.html">Actor</a></td></tr>
<tr class="table-series"><td><a href="/listpages/series_12_1.html">Series</a></td></tr>
<tr class="table-actor-type"><td><a href="/listpages/category_22_1.html?sort=pop">Category</a></td></tr>
<tr class="table-tag-keyword-small"><td><a href="/search/Keyword/1.html">Keyword</a></td></tr>
<tr class="table-tag-keyword-big"><td><a href="/search/Keyword/1.html?sort=pop">Keyword</a><a href="https://foreign.example/listpages/category_23_1.html">Foreign</a></td></tr>
<tr class="table-estimate"><td><span itemprop="ratingValue">4.5</span><span itemprop="ratingCount">20</span></td></tr>
</table></div>
<section id="section_gallery"><div class="sample-images">
<script>
if (isLogin()) {
  document.write('<a href="/member/contents/3000/1234/gallery/001.jpg"><img src="/member/contents/3000/1234/gallery/thumbnail_001.jpg"></a>');
} else {
  document.write('<a href="/contents/3000/1234/gallery/001.jpg"><img src="/contents/3000/1234/gallery/thumbnail_001.jpg"></a>');
  document.write('<a href="//www.heyzo.com/contents/3000/1234/gallery/002.jpg">Sample</a>');
  document.write('<img src="/contents/3000/1234/gallery/thumbnail_004.jpg">');
}
</script>
<a href="/contents/3000/1234/gallery/002.jpg">Duplicate</a>
<a href="/contents/3000/1234/gallery/003.jpg">Sample</a>
<a href="/contents/3000/1234/gallery/thumbnail_004.jpg">Thumbnail</a>
<a href="/contents/3000/4321/gallery/001.jpg">Different film</a>
<a href="https://heyzo.com.evil.example/contents/3000/1234/gallery/004.jpg">Foreign</a>
<a href="https://user@www.heyzo.com/contents/3000/1234/gallery/005.jpg">Credentials</a>
<a href="http://www.heyzo.com/contents/3000/1234/gallery/006.jpg">Insecure</a>
<a href="https://www.heyzo.com:8080/contents/3000/1234/gallery/007.jpg">Port</a>
<a href="javascript:alert(1)">Script</a>
</div></section>
<section><h1>Recommendations</h1><a href="/contents/3000/1234/gallery/099.jpg">Outside gallery</a></section>
</body></html>`

func TestHEYZOParsesIdentityMetadataAndPublicOriginals(t *testing.T) {
	m, err := parseHEYZO(heyzoPage, "https://www.heyzo.com/", "1234")
	if err != nil {
		t.Fatal(err)
	}
	d := m.Detail
	if d.ID != "" || d.Code != "HEYZO-1234" || d.Title != "Official & title" || d.OriginTitle != d.Title || d.Zone != domain.ZoneUncensored ||
		!reflect.DeepEqual(d.Sources, []domain.SourceID{{Provider: "heyzo", ID: "1234"}}) {
		t.Fatalf("wrong identity: %+v", d)
	}
	if d.ReleaseDate != "2026-03-15" || d.Duration != 61 || d.Rating != 4.5 || d.RatingMax != 5 || d.RatingSource != "heyzo" {
		t.Fatalf("wrong metadata: %+v", d)
	}
	if len(d.Actors) != 1 || d.Actors[0].Provider != "heyzo" || d.Actors[0].ID != "580" || d.Actors[0].Name != "Actor" ||
		d.Maker == nil || d.Maker.Name != "HEYZO" || d.Series == nil || d.Series.ID != "12" {
		t.Fatalf("wrong entities: %+v", d)
	}
	wantTags := []domain.Tag{{Provider: "heyzo", ID: "category:22", Name: "Category"}, {Provider: "heyzo", ID: "keyword:Keyword", Name: "Keyword"}}
	if !reflect.DeepEqual(d.Tags, wantTags) {
		t.Fatalf("wrong tags: %+v", d.Tags)
	}
	const prefix = "https://www.heyzo.com/contents/3000/1234/"
	wantImages := []domain.ImageCandidate{
		{Provider: "heyzo", Role: "cover", URL: prefix + "images/player_thumbnail.jpg", Layout: domain.CoverSingle},
		{Provider: "heyzo", Role: "preview", URL: prefix + "gallery/001.jpg"},
		{Provider: "heyzo", Role: "preview", URL: prefix + "gallery/002.jpg"},
		{Provider: "heyzo", Role: "preview", URL: prefix + "gallery/003.jpg"},
	}
	if !reflect.DeepEqual(m.Images, wantImages) || d.Cover != wantImages[0].URL || d.Thumbnail != d.Cover || len(d.PreviewImages) != 3 || !d.HasPreview {
		t.Fatalf("wrong artwork: %+v", m)
	}
	for i, preview := range d.PreviewImages {
		if preview.Original != wantImages[i+1].URL || preview.Thumbnail != preview.Original {
			t.Fatalf("preview not available to local details: %+v", preview)
		}
	}
	encoded, err := json.Marshal(m)
	if err != nil || bytes.Contains(encoded, []byte("Excluded synopsis")) {
		t.Fatal("unexpected synopsis or serialization error")
	}
}

func TestHEYZORejectsUnconfirmedIdentity(t *testing.T) {
	for _, identity := range []string{"", "//www.heyzo.com/moviepages/4321/index.html", "https://foreign.example/moviepages/1234/index.html", "https://www.heyzo.com/"} {
		t.Run(identity, func(t *testing.T) {
			body := strings.Replace(heyzoPage, "//www.heyzo.com/moviepages/1234/index.html", identity, 1)
			if _, err := parseHEYZO(body, "https://www.heyzo.com/", "1234"); err == nil {
				t.Fatal("unconfirmed page accepted")
			}
		})
	}
	body := strings.Replace(heyzoPage, "<h1>Official &amp; title</h1>", "", 1)
	if _, err := parseHEYZO(body, "https://www.heyzo.com/", "1234"); err == nil {
		t.Fatal("recommendation title accepted as movie title")
	}
}

func TestHEYZOOptionalFieldsAndCoverOwnership(t *testing.T) {
	for _, cover := range []string{"", "https://foreign.example/contents/3000/1234/images/player_thumbnail.jpg", "//www.heyzo.com/contents/3000/4321/images/player_thumbnail.jpg"} {
		body := `<meta property="og:url" content="https://www.heyzo.com/moviepages/1234/index.html"><meta property="og:image" content="` + cover + `"><div id="movie"><h1>Title</h1></div>`
		m, err := parseHEYZO(body, "https://www.heyzo.com/", "1234")
		if err != nil || m.Detail.Title != "Title" || m.Detail.Cover != "" || len(m.Images) != 0 || m.Detail.HasPreview {
			t.Fatalf("optional fields or cover ownership: %+v %v", m, err)
		}
	}
	body := strings.Replace(heyzoPage, "2026-03-15", "", 1)
	m, err := parseHEYZO(body, "https://www.heyzo.com/", "1234")
	if err != nil || m.Detail.ReleaseDate != "2026-03-14" {
		t.Fatalf("structured release date not used: %+v %v", m.Detail, err)
	}
}

func TestHEYZOFetchAndRouting(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet || r.Header.Get("User-Agent") == "" || r.Header.Get("Referer") == "" {
			t.Error("incorrect source request")
		}
		switch r.URL.Path {
		case "/moviepages/1234/index.html":
			_, _ = w.Write([]byte(strings.ReplaceAll(heyzoPage, `content="//`, `content="https://`)))
		case "/moviepages/9999/index.html":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	s := NewHEYZO(nil)
	s.base, s.http, s.limiter = server.URL+"/", server.Client(), rate.NewLimiter(rate.Inf, 1)
	defer s.Close()
	fanza := &FANZA{}
	if s.ID() != "heyzo" || !s.Supports("HEYZO-1234") || !s.Supports("HEYZO-0001") || fanza.Supports("HEYZO-1234") ||
		fanza.Supports("FC2-123456") || !fanza.Supports("ABP-123") {
		t.Fatal("HEYZO source routing failed")
	}
	for _, code := range []string{"ABP-123", "HEYDOUGA-1234-123", "HEYZO-1234A", "HEYZO-1234-2", "HEYZO-../1234", "HEYZO-"} {
		if s.Supports(code) {
			t.Fatalf("unsupported code %s", code)
		}
		if _, err := s.Fetch(t.Context(), domain.MovieRef{Code: code}); !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("invalid code fetched: %s %v", code, err)
		}
	}
	if calls != 0 {
		t.Fatal("invalid codes caused requests")
	}
	m, err := s.Fetch(t.Context(), domain.MovieRef{Code: "HEYZO-1234"})
	if err != nil || m.Detail.Code != "HEYZO-1234" {
		t.Fatalf("fetch failed: %+v %v", m, err)
	}
	if _, err := s.Fetch(t.Context(), domain.MovieRef{Code: "HEYZO-9999"}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("404 not mapped to source miss: %v", err)
	}
	if calls != 2 {
		t.Fatalf("unexpected requests: %d", calls)
	}
}

func TestHEYZOLive(t *testing.T) {
	codes := os.Getenv("MIYABI_TEST_HEYZO_CODES")
	if codes == "" {
		t.Skip("set MIYABI_TEST_HEYZO_CODES for live verification; images are decoded in memory")
	}
	proxy, err := netx.NewProxyManager(netx.ProxyConfig{Enabled: os.Getenv("MIYABI_TEST_PROXY") != "", URL: os.Getenv("MIYABI_TEST_PROXY")})
	if err != nil {
		t.Fatal(err)
	}
	s := NewHEYZO(proxy)
	defer s.Close()
	for _, code := range strings.Split(codes, ",") {
		t.Run(code, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
			defer cancel()
			m, err := s.Fetch(ctx, domain.MovieRef{Code: code})
			if err != nil {
				t.Fatal(err)
			}
			if m.Detail.Code != code || m.Detail.ID != "" || m.Detail.Title == "" || m.Detail.Duration == 0 || m.Detail.ReleaseDate == "" || len(m.Images) == 0 {
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
