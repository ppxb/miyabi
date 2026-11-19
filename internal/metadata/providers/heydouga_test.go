package providers

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
)

const heydougaDetailPage = `<html><head>
<meta property="og:url" content="https://www.heydouga.com/moviepages/4030/1938/index.html">
<meta property="og:image" content="https://image01-www.heydouga.com/contents/4030/1938/player_thumb.webp">
</head><body>
<div id="title-bg"><h1>サンプル作品タイトル</h1></div>
<div id="movie-description">あらすじ本文です。</div>
<ul>
<li><span>配信日：</span>2016-11-23</li>
<li><span>主演：</span>立花美涼</li>
<li><span>提供元：</span>av9898（公式サイト）</li>
<li><span>動画再生時間：</span>120分</li>
</ul>
</body></html>`

func TestHeydougaParsesDetail(t *testing.T) {
	m, err := parseHeydouga(heydougaDetailPage, "https://www.heydouga.com/", "4030", "1938")
	if err != nil {
		t.Fatal(err)
	}
	if m.Detail.Code != "HEYDOUGA-4030-1938" || m.Detail.Title != "サンプル作品タイトル" {
		t.Fatalf("identity/title wrong: %+v", m.Detail)
	}
	if m.Detail.ReleaseDate != "2016-11-23" || m.Detail.Duration != 120 {
		t.Fatalf("release/duration wrong: %+v", m.Detail)
	}
	if len(m.Detail.Actors) != 1 || m.Detail.Actors[0].Name != "立花美涼" {
		t.Fatalf("actors wrong: %+v", m.Detail.Actors)
	}
	if m.Detail.Maker == nil || m.Detail.Maker.Name != "av9898" {
		t.Fatalf("maker wrong: %+v", m.Detail.Maker)
	}
	if m.Detail.Cover != "https://image01-www.heydouga.com/contents/4030/1938/player_thumb.webp" {
		t.Fatalf("cover wrong: %q", m.Detail.Cover)
	}
	if !strings.Contains(m.Detail.Plot, "あらすじ") {
		t.Fatalf("plot wrong: %q", m.Detail.Plot)
	}
}

func TestHeydougaRejectsForeignCoverAndSupports(t *testing.T) {
	body := strings.Replace(heydougaDetailPage, "https://image01-www.heydouga.com", "https://foreign.example", 1)
	m, err := parseHeydouga(body, "https://www.heydouga.com/", "4030", "1938")
	if err != nil {
		t.Fatal(err)
	}
	if m.Detail.Cover != "" {
		t.Fatalf("foreign cover accepted: %q", m.Detail.Cover)
	}
	if _, err := parseHeydouga(`<div id="title-bg"><h1></h1></div>`, "https://www.heydouga.com/", "4030", "1938"); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("empty title accepted: %v", err)
	}
	p := &Heydouga{}
	if p.Supports("SSIS-033") {
		t.Fatal("should not support SSIS-033")
	}
	if _, err := p.Fetch(t.Context(), domain.MovieRef{Code: "SSIS-033"}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("unsupported fetched: %v", err)
	}
	if !p.Supports("HEYDOUGA-4030-1938") {
		t.Fatal("should support heydouga id")
	}
}

func TestHeydougaRejectsWrongIdentity(t *testing.T) {
	// og:url pointing at another work must be rejected even with a title present.
	body := strings.Replace(heydougaDetailPage, "/moviepages/4030/1938/", "/moviepages/4030/1999/", 1)
	if _, err := parseHeydouga(body, "https://www.heydouga.com/", "4030", "1938"); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("wrong identity accepted: %v", err)
	}
}

func TestHeydougaLive(t *testing.T) {
	codes := os.Getenv("MIYABI_TEST_HEYDOUGA_CODES")
	if codes == "" {
		t.Skip("set MIYABI_TEST_HEYDOUGA_CODES for live verification")
	}
	proxy, err := netx.NewProxyManager(netx.ProxyConfig{Enabled: os.Getenv("MIYABI_TEST_PROXY") != "", URL: os.Getenv("MIYABI_TEST_PROXY")})
	if err != nil {
		t.Fatal(err)
	}
	s := NewHeydouga(proxy)
	defer s.Close()
	for _, code := range strings.Split(codes, ",") {
		t.Run(code, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
			defer cancel()
			m, err := s.Fetch(ctx, domain.MovieRef{Code: code})
			if err != nil {
				t.Fatal(err)
			}
			if m.Detail.Code == "" || m.Detail.Title == "" || m.Detail.Cover == "" {
				t.Fatalf("missing core metadata: %+v", m.Detail)
			}
			t.Logf("%s: release %q dur %d actors %d maker %v", code, m.Detail.ReleaseDate, m.Detail.Duration, len(m.Detail.Actors), m.Detail.Maker)
		})
	}
}
