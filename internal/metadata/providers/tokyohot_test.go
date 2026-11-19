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

const tokyoHotDetailPage = `<html><body><div id="main">
<div class="contents"><h2>サンプル作品</h2></div>
<div class="infowrapper"><dl>
<dt>出演者</dt><dd><a href="/star/1">女優A</a><a href="/star/2">女優B</a></dd>
<dt>プレイ内容</dt><dd><a href="#">企画</a></dd>
<dt>タグ</dt><dd><a href="#">タグA</a></dd>
<dt>シリーズ</dt><dd>シリーズX</dd>
<dt>レーベル</dt><dd>レーベルY</dd>
<dt>配信開始日</dt><dd>2024/05/01</dd>
<dt>収録時間</dt><dd>120分</dd>
<dt>作品番号</dt><dd>n1633</dd>
</dl></div>
<a href="https://my.cdn.tokyo-hot.com/media/n1633/jacket/n1633.jpg">c</a>
<a href="https://my.cdn.tokyo-hot.com/media/n1633/package/_v.jpg">t</a>
<a rel="cap" href="https://my.cdn.tokyo-hot.com/media/n1633/vcap/1.jpg">p1</a>
<a rel="cap" href="https://my.cdn.tokyo-hot.com/media/n1633/vcap/2.jpg">p2</a>
<a rel="cap" href="https://foreign.example/x.jpg">bad</a>
</div></body></html>`

func TestTokyoHotParsesDetail(t *testing.T) {
	m, err := parseTokyoHot(tokyoHotDetailPage, "https://my.tokyo-hot.com/", "N1633")
	if err != nil {
		t.Fatal(err)
	}
	if m.Detail.Code != "N1633" || m.Detail.Title != "サンプル作品" || m.Detail.Zone != domain.ZoneUncensored {
		t.Fatalf("identity/zone wrong: %+v", m.Detail)
	}
	if m.Detail.ReleaseDate != "2024-05-01" || m.Detail.Duration != 120 {
		t.Fatalf("release/duration wrong: %+v", m.Detail)
	}
	if len(m.Detail.Actors) != 2 || m.Detail.Actors[0].Name != "女優A" {
		t.Fatalf("actors wrong: %+v", m.Detail.Actors)
	}
	if len(m.Detail.Tags) != 2 {
		t.Fatalf("tags wrong: %+v", m.Detail.Tags)
	}
	if m.Detail.Series == nil || m.Detail.Series.Name != "シリーズX" || m.Detail.Maker == nil || m.Detail.Maker.Name != "TOKYO-HOT" {
		t.Fatalf("series/maker wrong: %+v", m.Detail)
	}
	if m.Detail.Cover != "https://my.cdn.tokyo-hot.com/media/n1633/jacket/n1633.jpg" ||
		m.Detail.Thumbnail != "https://my.cdn.tokyo-hot.com/media/n1633/package/_v.jpg" {
		t.Fatalf("cover/thumb wrong: %+v", m.Detail)
	}
	if len(m.Detail.PreviewImages) != 2 {
		t.Fatalf("preview wrong (foreign host must be dropped): %+v", m.Detail.PreviewImages)
	}
}

func TestTokyoHotSoftNotFoundAndUnsupported(t *testing.T) {
	if _, err := parseTokyoHot(`<div id="main"><div class="contents"><h2>t</h2></div></div>`, "https://my.tokyo-hot.com/", "N1633"); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("soft 404 accepted: %v", err)
	}
	p := &TokyoHot{}
	for _, code := range []string{"SSIS-033", "ABC", ""} {
		if p.Supports(code) {
			t.Fatalf("should not support %q", code)
		}
		if _, err := p.Fetch(t.Context(), domain.MovieRef{Code: code}); !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("unsupported fetched: %s %v", code, err)
		}
	}
	for _, code := range []string{"N1633", "N-1633", "K1234", "KB1624", "S2MBD-002", "5656"} {
		if !p.Supports(code) {
			t.Fatalf("should support %q", code)
		}
	}
}

func TestTokyoHotDurationAndID(t *testing.T) {
	cases := map[string]int{"01:38:38": 98, "00:58:03": 58, "1時間20分": 80, "120分": 120}
	for in, want := range cases {
		if got := japaneseDuration(in); got != want {
			t.Fatalf("japaneseDuration(%q)=%d want %d", in, got, want)
		}
	}
	ids := map[string]string{"N1633": "n1633", "N-1633": "n1633", "KB1624": "kb1624", "KB-1624": "kb1624", "S2MBD-002": "s2mbd-002", "5656": "5656", "SSIS-033": ""}
	for in, want := range ids {
		if got := tokyoHotID(in); got != want {
			t.Fatalf("tokyoHotID(%q)=%q want %q", in, got, want)
		}
	}
}

func TestTokyoHotLive(t *testing.T) {
	codes := os.Getenv("MIYABI_TEST_TOKYOHOT_CODES")
	if codes == "" {
		t.Skip("set MIYABI_TEST_TOKYOHOT_CODES for live verification")
	}
	proxy, err := netx.NewProxyManager(netx.ProxyConfig{Enabled: os.Getenv("MIYABI_TEST_PROXY") != "", URL: os.Getenv("MIYABI_TEST_PROXY")})
	if err != nil {
		t.Fatal(err)
	}
	s := NewTokyoHot(proxy)
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
			t.Logf("%s: release %q dur %d actors %d tags %d previews %d", code, m.Detail.ReleaseDate, m.Detail.Duration, len(m.Detail.Actors), len(m.Detail.Tags), len(m.Detail.PreviewImages))
		})
	}
}
