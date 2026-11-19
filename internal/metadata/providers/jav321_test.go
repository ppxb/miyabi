package providers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/time/rate"
)

func newJav321TestServer(t *testing.T, status int, location string) (*Jav321, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if location != "" {
			w.Header().Set("Location", location)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte("x"))
	}))
	s := &Jav321{client: client{http: srv.Client(), limiter: rate.NewLimiter(rate.Inf, 1), base: srv.URL + "/"}}
	return s, srv.Close
}

func TestJav321ResolveDetailURL(t *testing.T) {
	ok, stop := newJav321TestServer(t, http.StatusMovedPermanently, "https://www.jav321.com/video/ssis00033")
	defer stop()
	if got, err := ok.resolveDetailURL(t.Context(), "SSIS-033"); err != nil || got != "https://www.jav321.com/video/ssis00033" {
		t.Fatalf("resolve ok: got %q err %v", got, err)
	}

	for name, tc := range map[string]struct {
		status   int
		location string
	}{
		"no-redirect":  {http.StatusOK, ""},
		"foreign-host": {http.StatusMovedPermanently, "https://evil.example/video/x"},
		"non-video":    {http.StatusMovedPermanently, "https://www.jav321.com/star/1"},
		"http-scheme":  {http.StatusMovedPermanently, "http://www.jav321.com/video/x"},
	} {
		s, stop := newJav321TestServer(t, tc.status, tc.location)
		got, err := s.resolveDetailURL(t.Context(), "SSIS-033")
		stop()
		if !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("%s: want ErrNotFound, got %q %v", name, got, err)
		}
	}

	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError} {
		s, stop := newJav321TestServer(t, status, "")
		_, err := s.resolveDetailURL(t.Context(), "SSIS-033")
		stop()
		if err == nil || errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("status %d should be retryable, got %v", status, err)
		}
	}
}

const jav321DetailPage = `<html><body>
<div class="panel-heading"><h3>サンプル作品タイトル</h3></div>
<div class="panel-body">
<b>出演者</b>:
<a href="/star/1063214/1">七ツ森りり</a>
<a href="/star/1063214/1">七ツ森りり</a>
<b>メーカー</b><a href="/company/%E3%82%A8%E3%82%B9%E3%83%AF%E3%83%B3/1">エスワン</a>
<b>品番</b>: ssis-033
<b>配信開始日</b>: 2021-04-07
<b>収録時間</b>: 169 minutes
<b>平均評価</b>: 4.5
<div class="col-xs-12 col-md-12"><p><a href="#x"><img src="http://pics.dmm.co.jp//digital/video/ssis00033/ssis00033pl.jpg"></a></p></div>
<div class="col-xs-12 col-md-12"><p><a href="#y"><img src="http://pics.dmm.co.jp/digital/video/ssis00033/ssis00033jp-1.jpg"></a></p></div>
</div></body></html>`

func TestJav321ParsesDetail(t *testing.T) {
	m, err := parseJav321(jav321DetailPage, "https://www.jav321.com/", "SSIS-033")
	if err != nil {
		t.Fatal(err)
	}
	if m.Detail.Code != "SSIS-033" || m.Detail.Title != "サンプル作品タイトル" {
		t.Fatalf("identity/title wrong: %+v", m.Detail)
	}
	if m.Detail.ReleaseDate != "2021-04-07" || m.Detail.Duration != 169 {
		t.Fatalf("release/duration wrong: %+v", m.Detail)
	}
	if m.Detail.Rating != 4.5 || m.Detail.RatingMax != 5 || m.Detail.RatingSource != "jav321" {
		t.Fatalf("rating wrong: %+v", m.Detail)
	}
	if len(m.Detail.Actors) != 1 || m.Detail.Actors[0].ID != "1063214" || m.Detail.Actors[0].Name != "七ツ森りり" {
		t.Fatalf("actors wrong: %+v", m.Detail.Actors)
	}
	if m.Detail.Maker == nil || m.Detail.Maker.Name != "エスワン" {
		t.Fatalf("maker wrong: %+v", m.Detail.Maker)
	}
	if m.Detail.Cover != "https://www.jav321.com/digital/video/ssis00033/ssis00033pl.jpg" {
		t.Fatalf("cover wrong: %q", m.Detail.Cover)
	}
	if len(m.Images) != 2 || m.Images[0].Role != "cover" || m.Images[1].Role != "preview" {
		t.Fatalf("images wrong: %+v", m.Images)
	}
	if !m.Detail.HasPreview || len(m.Detail.PreviewImages) != 1 {
		t.Fatalf("preview wrong: %+v", m.Detail.PreviewImages)
	}
}

func TestJav321RejectsMismatchedNumber(t *testing.T) {
	body := strings.Replace(jav321DetailPage, "<b>品番</b>: ssis-033", "<b>品番</b>: midv-005", 1)
	if _, err := parseJav321(body, "https://www.jav321.com/", "SSIS-033"); err == nil {
		t.Fatal("mismatched number accepted")
	}
}

func TestJav321SupportsAndRejectsSpecialFamilies(t *testing.T) {
	p := &Jav321{}
	for _, code := range []string{"FC2-123", "HEYZO-123", ""} {
		if p.Supports(code) {
			t.Fatalf("supports special family: %s", code)
		}
		if _, err := p.Fetch(t.Context(), domain.MovieRef{Code: code}); !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("special family fetched: %s %v", code, err)
		}
	}
	if !p.Supports("SSIS-033") {
		t.Fatal("should support a standard number")
	}
}

func TestJav321MirrorHostWhitelist(t *testing.T) {
	base := "https://www.jav321.com/"
	cases := map[string]string{
		"http://pics.dmm.co.jp//digital/video/x/a.jpg": "https://www.jav321.com/digital/video/x/a.jpg",
		"http://pics.dmm.co.jp/digital/video/x/a.jpg":  "https://www.jav321.com/digital/video/x/a.jpg",
		"https://www.jav321.com/digital/video/x/a.jpg": "https://www.jav321.com/digital/video/x/a.jpg",
		"https://foreign.example/x/a.jpg":              "",
		"":                                             "",
	}
	for in, want := range cases {
		if got := jav321Mirror(base, in); got != want {
			t.Fatalf("jav321Mirror(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJav321Live(t *testing.T) {
	codes := os.Getenv("MIYABI_TEST_JAV321_CODES")
	if codes == "" {
		t.Skip("set MIYABI_TEST_JAV321_CODES for live verification")
	}
	proxy, err := netx.NewProxyManager(netx.ProxyConfig{Enabled: os.Getenv("MIYABI_TEST_PROXY") != "", URL: os.Getenv("MIYABI_TEST_PROXY")})
	if err != nil {
		t.Fatal(err)
	}
	s := NewJav321(proxy)
	defer s.Close()
	for _, code := range strings.Split(codes, ",") {
		t.Run(code, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
			defer cancel()
			m, err := s.Fetch(ctx, domain.MovieRef{Code: code})
			if err != nil {
				t.Fatal(err)
			}
			if m.Detail.Code == "" || m.Detail.Title == "" || m.Detail.Cover == "" || len(m.Images) == 0 {
				t.Fatalf("missing core metadata: %+v", m.Detail)
			}
			t.Logf("%s: release %q, dur %d, actors %d, images %d", code, m.Detail.ReleaseDate, m.Detail.Duration, len(m.Detail.Actors), len(m.Images))
		})
	}
}
