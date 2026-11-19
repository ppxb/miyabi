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

const dahliaPage = `<html><body>
<div class="bar02_works"><h1>サンプル作品</h1></div>
<div class="box_works01_text">あらすじ本文</div>
<div class="box_works01_list"><ul>
<li><span>出演女優</span><p>女優A / 女優B</p></li>
<li><span>収録時間</span><p>120分</p></li>
<li><span>配信開始日</span><p>2024-03-01</p></li>
</ul></div>
<div class="box_works01_img"><a href="https://cdn.faleno.net/x/v.mp4"><img src="https://cdn.faleno.net/x/cover.jpg?w=100"></a></div>
<div class="box_works01_ga"><ul><li><a href="https://cdn.faleno.net/x/1.jpg">p</a></li><li><a href="https://foreign.example/x.jpg">bad</a></li></ul></div>
</body></html>`

func TestDahliaParsesDetail(t *testing.T) {
	m, err := parseDahlia(dahliaPage, "https://dahlia-av.jp/", "DLDSS-339", "dldss339", "dahlia", "DAHLIA")
	if err != nil {
		t.Fatal(err)
	}
	if m.Detail.Code != "DLDSS-339" || m.Detail.Title != "サンプル作品" || m.Detail.Plot != "あらすじ本文" || m.Detail.Zone != domain.ZoneCensored {
		t.Fatalf("identity/plot/zone wrong: %+v", m.Detail)
	}
	if m.Detail.Duration != 120 || m.Detail.ReleaseDate != "2024-03-01" {
		t.Fatalf("duration/release wrong: %+v", m.Detail)
	}
	if len(m.Detail.Actors) != 2 {
		t.Fatalf("actors wrong: %+v", m.Detail.Actors)
	}
	if m.Detail.Maker == nil || m.Detail.Maker.Name != "DAHLIA" {
		t.Fatalf("maker wrong: %+v", m.Detail.Maker)
	}
	if m.Detail.Cover != "https://cdn.faleno.net/x/cover.jpg" {
		t.Fatalf("cover wrong (query must be stripped): %q", m.Detail.Cover)
	}
	if len(m.Detail.PreviewImages) != 1 {
		t.Fatalf("preview wrong (foreign dropped): %+v", m.Detail.PreviewImages)
	}
}

func TestDahliaRejectsWrongPageAndSupports(t *testing.T) {
	// og:url pointing at another work must be rejected.
	body := strings.Replace(dahliaPage, "<body>",
		`<head><meta property="og:url" content="https://dahlia-av.jp/works/dldss999/"></head><body>`, 1)
	if _, err := parseDahlia(body, "https://dahlia-av.jp/", "DLDSS-339", "dldss339", "dahlia", "DAHLIA"); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("wrong og:url accepted: %v", err)
	}
	p := &Dahlia{}
	if !p.Supports("DLDSS-339") || !p.Supports("dldss339") || p.Supports("SSIS-033") {
		t.Fatal("dahlia Supports wrong")
	}
	if _, err := p.Fetch(t.Context(), domain.MovieRef{Code: "SSIS-033"}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("unsupported fetched: %v", err)
	}
}

const dugaPage = `<html><head>
<script type="application/ld+json">{"description":"あらすじ"}</script>
</head><body>
<div id="contentsname">サンプル作品</div>
<div class="imagebox"><a href="https://img.duga.jp/x/cover.jpg"><img id="productjpg" src="https://img.duga.jp/x/thumb.jpg"></a></div>
<table>
<tr><th>配信開始日</th><td>2023-05-01</td></tr>
<tr><th>メーカー</th><td>メーカーX</td></tr>
<tr><th>シリーズ</th><td>シリーズY</td></tr>
<tr><th>再生時間</th><td>01:30:00</td></tr>
</table>
<ul class="performer"><li><a>女優A</a></li><li><a>女優B</a></li></ul>
<ul class="categorylist"><li><a>タグA</a></li></ul>
<ul class="director"><li><a>監督A</a></li></ul>
<ul id="digestthumbbox"><li><a href="https://pic.duga.jp/x/1.jpg">p</a></li><li><a href="https://foreign.example/x.jpg">b</a></li></ul>
</body></html>`

func TestDugaParsesDetail(t *testing.T) {
	m, err := parseDuga(dugaPage, "https://duga.jp/", "GLORY-4262", "glory-4262", "duga")
	if err != nil {
		t.Fatal(err)
	}
	d := m.Detail
	if d.Code != "GLORY-4262" || d.Title != "サンプル作品" || d.Plot != "あらすじ" || d.Zone != domain.ZoneCensored {
		t.Fatalf("identity/plot/zone wrong: %+v", d)
	}
	if d.ReleaseDate != "2023-05-01" || d.Duration != 90 {
		t.Fatalf("release/duration wrong: %+v", d)
	}
	if len(d.Actors) != 2 || len(d.Tags) != 1 || d.Director == nil || d.Maker == nil || d.Series == nil {
		t.Fatalf("actors/tags/director/maker/series wrong: %+v", d)
	}
	if d.Cover != "https://img.duga.jp/x/cover.jpg" || len(d.PreviewImages) != 1 {
		t.Fatalf("cover/preview wrong: %q %+v", d.Cover, d.PreviewImages)
	}
}

func TestDugaReleasePriorityAndJapaneseDate(t *testing.T) {
	// 発売日 appears before 配信開始日 and differs: must prefer 配信開始日.
	body := `<div id="contentsname">t</div><table>
<tr><th>発売日</th><td>2021年11月16日</td></tr>
<tr><th>配信開始日</th><td>2022年06月06日</td></tr></table>`
	m, err := parseDuga(body, "https://duga.jp/", "GLORY-4262", "glory-4262", "duga")
	if err != nil {
		t.Fatal(err)
	}
	if m.Detail.ReleaseDate != "2022-06-06" {
		t.Fatalf("release priority/japanese date wrong: %q", m.Detail.ReleaseDate)
	}
}

func TestDugaRejectsSoftPageAndWrongIdentity(t *testing.T) {
	if _, err := parseDuga(`<div>no contentsname</div>`, "https://duga.jp/", "GLORY-4262", "glory-4262", "duga"); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("soft page accepted: %v", err)
	}
	body := `<head><meta property="og:url" content="https://duga.jp/ppv/other-999/"></head><body><div id="contentsname">t</div></body>`
	if _, err := parseDuga(body, "https://duga.jp/", "GLORY-4262", "glory-4262", "duga"); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("wrong og:url accepted: %v", err)
	}
}

func TestDugaSupports(t *testing.T) {
	p := &Duga{}
	if !p.Supports("GLORY-4262") || !p.Supports("WAAP-1294") {
		t.Fatal("duga should support label codes")
	}
	if p.Supports("FC2-123") || p.Supports("HEYZO-123") {
		t.Fatal("duga must exclude FC2/HEYZO")
	}
	if _, err := p.Fetch(t.Context(), domain.MovieRef{Code: "FC2-123"}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("excluded fetched: %v", err)
	}
}

func liveP2(t *testing.T, env string, newSrc func(*netx.ProxyManager) interface {
	Fetch(context.Context, domain.MovieRef) (domain.MovieMetadata, error)
	Close()
}) {
	t.Helper()
	codes := os.Getenv(env)
	if codes == "" {
		t.Skip("set " + env + " for live verification")
	}
	proxy, err := netx.NewProxyManager(netx.ProxyConfig{Enabled: os.Getenv("MIYABI_TEST_PROXY") != "", URL: os.Getenv("MIYABI_TEST_PROXY")})
	if err != nil {
		t.Fatal(err)
	}
	s := newSrc(proxy)
	defer s.Close()
	for _, c := range strings.Split(codes, ",") {
		ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
		m, err := s.Fetch(ctx, domain.MovieRef{Code: c})
		cancel()
		if err != nil {
			t.Fatalf("%s: %v", c, err)
		}
		if m.Detail.Code == "" || m.Detail.Title == "" || m.Detail.Cover == "" {
			t.Fatalf("%s missing core: %+v", c, m.Detail)
		}
		t.Logf("%s: release %q dur %d actors %d", c, m.Detail.ReleaseDate, m.Detail.Duration, len(m.Detail.Actors))
	}
}

func TestDahliaLive(t *testing.T) {
	liveP2(t, "MIYABI_TEST_DAHLIA_CODES", func(p *netx.ProxyManager) interface {
		Fetch(context.Context, domain.MovieRef) (domain.MovieMetadata, error)
		Close()
	} {
		return NewDahlia(p)
	})
}

func TestDugaLive(t *testing.T) {
	liveP2(t, "MIYABI_TEST_DUGA_CODES", func(p *netx.ProxyManager) interface {
		Fetch(context.Context, domain.MovieRef) (domain.MovieMetadata, error)
		Close()
	} {
		return NewDuga(p)
	})
}
