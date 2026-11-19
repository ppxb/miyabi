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

const fc2dbWorkPage = `<html><head>
<meta property="og:url" content="https://fc2db.net/work/1234567/">
<meta property="og:image" content="https://img.fc2db.net/wp-content/uploads/2026/05/02164443/1234567.webp">
</head><body>
<h1>[FC2-PPV-1234567] サンプル作品タイトル</h1>
<dl class="grid">
<dt>販売日</dt><dd>2026/03/22</dd>
<dt>収録時間</dt><dd>84</dd>
<dt>モザイク</dt><dd>有</dd>
<dt>商品ID</dt><dd>1234567</dd>
<dt>作品販売者</dt><dd>テスト販売者</dd>
</dl>
<div class="actresses"><span>出演女優：</span><a href="https://fc2db.net/actress/1321/">女優A</a><a href="https://fc2db.net/actress/1619/">女優B</a></div>
<p><a href="https://fc2db.net/work-tags/%e3%83%8f%e3%83%a1%e6%92%ae%e3%82%8a/">ハメ撮り</a></p>
</body></html>`

func TestFC2DBParsesWorkFields(t *testing.T) {
	m, err := parseFC2DB(fc2dbWorkPage, "https://fc2db.net/", "1234567")
	if err != nil {
		t.Fatal(err)
	}
	if m.Detail.Code != "FC2-1234567" || m.Detail.Title != "サンプル作品タイトル" || m.Detail.OriginTitle != "サンプル作品タイトル" {
		t.Fatalf("identity/title wrong: %+v", m.Detail)
	}
	if m.Detail.ReleaseDate != "2026-03-22" {
		t.Fatalf("release date wrong: %q", m.Detail.ReleaseDate)
	}
	if m.Detail.Maker == nil || m.Detail.Maker.Name != "テスト販売者" {
		t.Fatalf("maker wrong: %+v", m.Detail.Maker)
	}
	if len(m.Detail.Actors) != 2 || m.Detail.Actors[0].ID != "1321" || m.Detail.Actors[1].Name != "女優B" {
		t.Fatalf("actors wrong: %+v", m.Detail.Actors)
	}
	if len(m.Detail.Tags) != 1 || m.Detail.Tags[0].ID != "ハメ撮り" || m.Detail.Tags[0].Name != "ハメ撮り" {
		t.Fatalf("tags wrong: %+v", m.Detail.Tags)
	}
	if m.Detail.Cover != "https://img.fc2db.net/wp-content/uploads/2026/05/02164443/1234567.webp" {
		t.Fatalf("cover wrong: %q", m.Detail.Cover)
	}
	if len(m.Images) != 1 || m.Images[0].Role != "cover" || m.Images[0].Layout != domain.CoverSingle {
		t.Fatalf("images wrong: %+v", m.Images)
	}
}

func TestFC2DBRejectsUnconfirmedAndMismatched(t *testing.T) {
	cases := []string{
		// product ID in the detail list disagrees with the requested number.
		strings.Replace(fc2dbWorkPage, "<dt>商品ID</dt><dd>1234567</dd>", "<dt>商品ID</dt><dd>7654321</dd>", 1),
		// foreign og:url, no h1 prefix and no product ID row: nothing confirms it.
		`<meta property="og:url" content="https://foreign.example/work/1234567/">
		 <h1>Unconfirmed title</h1>`,
		// h1 code prefix disagrees with the requested number.
		strings.Replace(fc2dbWorkPage, "[FC2-PPV-1234567]", "[FC2-PPV-7654321]", 1),
	}
	for _, body := range cases {
		if _, err := parseFC2DB(body, "https://fc2db.net/", "1234567"); err == nil {
			t.Fatal("unconfirmed work accepted")
		}
	}
	for _, code := range []string{"", "ABP-123", "FC2-PPV-../1234567", "FC2-1234567A"} {
		if _, err := (&FC2DB{}).Fetch(t.Context(), domain.MovieRef{Code: code}); !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("invalid code fetched: %s %v", code, err)
		}
	}
}

func TestFC2DBImageHostWhitelist(t *testing.T) {
	cases := map[string]string{
		"https://img.fc2db.net/wp-content/uploads/a.webp": "https://img.fc2db.net/wp-content/uploads/a.webp",
		"https://fc2db.net/x/a.webp":                      "https://fc2db.net/x/a.webp",
		"//img.fc2db.net/a.webp":                          "https://img.fc2db.net/a.webp",
		"https://foreign.example/a.webp":                  "",
		"http://img.fc2db.net/a.webp":                     "",
		"":                                                "",
	}
	for in, want := range cases {
		if got := fc2dbImage("https://fc2db.net/", in); got != want {
			t.Fatalf("fc2dbImage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFC2DBRejectsForeignCover(t *testing.T) {
	body := strings.Replace(fc2dbWorkPage,
		"https://img.fc2db.net/wp-content/uploads/2026/05/02164443/1234567.webp",
		"https://foreign.example/cover.webp", 1)
	m, err := parseFC2DB(body, "https://fc2db.net/", "1234567")
	if err != nil {
		t.Fatal(err)
	}
	if m.Detail.Cover != "" || len(m.Images) != 0 {
		t.Fatalf("foreign cover accepted: %+v", m)
	}
}

func TestFC2DBLive(t *testing.T) {
	codes := os.Getenv("MIYABI_TEST_FC2DB_CODES")
	if codes == "" {
		t.Skip("set MIYABI_TEST_FC2DB_CODES for live verification")
	}
	proxy, err := netx.NewProxyManager(netx.ProxyConfig{Enabled: os.Getenv("MIYABI_TEST_PROXY") != "", URL: os.Getenv("MIYABI_TEST_PROXY")})
	if err != nil {
		t.Fatal(err)
	}
	s := NewFC2DB(proxy)
	defer s.Close()
	for _, code := range strings.Split(codes, ",") {
		t.Run(code, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
			defer cancel()
			m, err := s.Fetch(ctx, domain.MovieRef{Code: code})
			if err != nil {
				t.Fatal(err)
			}
			if m.Detail.Code == "" || m.Detail.Title == "" {
				t.Fatalf("missing identity: %+v", m.Detail)
			}
			t.Logf("%s: release %q, cover=%v, actors %d, tags %d", code, m.Detail.ReleaseDate, m.Detail.Cover != "", len(m.Detail.Actors), len(m.Detail.Tags))
		})
	}
}
