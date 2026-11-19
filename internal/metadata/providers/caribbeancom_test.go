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

const caribbeancomPage = `<html><body>
<h1 itemprop="name">サンプル作品</h1>
<p itemprop="description">あらすじ本文。</p>
<div id="moviepages"><ul>
<li><span>出演</span><span><a href="/actor/1">女優A</a><a href="/actor/2">女優B</a></span></li>
<li><span>配信日</span><span>2022/05/04</span></li>
<li><span>再生時間</span><span>01:20:00</span></li>
<li><span>タグ</span><span><a href="/tag/1">単体作品</a></span></li>
<li><span>スタジオ</span><span><a href="/studio/1">スタジオX</a></span></li>
</ul></div>
<script>var emimg = 'https://www.caribbeancom.com/moviepages/050422-001/images/l.jpg';</script>
<div class="gallery-ratio"><a href="https://www.caribbeancom.com/moviepages/050422-001/images/1.jpg">p</a><a href="https://www.caribbeancom.com/member/x.jpg">m</a></div>
</body></html>`

func TestCaribbeanParsesDetail(t *testing.T) {
	m, err := parseCaribbean(caribbeancomPage, "https://www.caribbeancom.com/", "050422-001", "050422-001", "caribbeancom", "カリビアンコム")
	if err != nil {
		t.Fatal(err)
	}
	if m.Detail.Code != "050422-001" || m.Detail.Title != "サンプル作品" || m.Detail.Zone != domain.ZoneUncensored {
		t.Fatalf("identity/zone wrong: %+v", m.Detail)
	}
	if m.Detail.ReleaseDate != "2022-05-04" || m.Detail.Duration != 80 {
		t.Fatalf("release/duration wrong: %+v", m.Detail)
	}
	if len(m.Detail.Actors) != 2 || m.Detail.Actors[0].Name != "女優A" {
		t.Fatalf("actors wrong: %+v", m.Detail.Actors)
	}
	if len(m.Detail.Tags) != 1 || m.Detail.Tags[0].Name != "単体作品" {
		t.Fatalf("tags wrong: %+v", m.Detail.Tags)
	}
	if m.Detail.Maker == nil || m.Detail.Maker.Name != "スタジオX" {
		t.Fatalf("maker wrong: %+v", m.Detail.Maker)
	}
	if m.Detail.Cover != "https://www.caribbeancom.com/moviepages/050422-001/images/l.jpg" {
		t.Fatalf("cover wrong: %q", m.Detail.Cover)
	}
	if !m.Detail.HasPreview || len(m.Detail.PreviewImages) != 1 {
		t.Fatalf("preview wrong (member must drop): %+v", m.Detail.PreviewImages)
	}
}

func TestCaribbeanDateFallbackAndRejects(t *testing.T) {
	body := `<h1 itemprop="name">t</h1><div id="moviepages"><ul></ul></div><script>var id='050422-001';</script>`
	m, err := parseCaribbean(body, "https://www.caribbeancom.com/", "050422-001", "050422-001", "caribbeancom", "x")
	if err != nil {
		t.Fatal(err)
	}
	if m.Detail.ReleaseDate != "2022-05-04" {
		t.Fatalf("date fallback wrong: %q", m.Detail.ReleaseDate)
	}
	// A title but no #moviepages detail container must be rejected.
	if _, err := parseCaribbean(`<h1 itemprop="name">t</h1>`, "https://www.caribbeancom.com/", "050422-001", "050422-001", "caribbeancom", "x"); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("missing container accepted: %v", err)
	}
	p := &Caribbeancom{}
	if !p.Supports("050422-001") || !p.Supports("050422_001") || p.Supports("SSIS-033") {
		t.Fatal("caribbeancom Supports wrong")
	}
	if _, err := p.Fetch(t.Context(), domain.MovieRef{Code: "SSIS-033"}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("unsupported fetched: %v", err)
	}
	if q := (&CaribbeancomPremium{}); !q.Supports("052121_002") || q.Supports("SSIS-033") {
		t.Fatal("caribbeancompr Supports wrong")
	}
}

func TestCaribbeanLive(t *testing.T) {
	codes := os.Getenv("MIYABI_TEST_CARIBBEAN_COM_CODES")
	if codes == "" {
		t.Skip("set MIYABI_TEST_CARIBBEAN_COM_CODES for live verification")
	}
	proxy, err := netx.NewProxyManager(netx.ProxyConfig{Enabled: os.Getenv("MIYABI_TEST_PROXY") != "", URL: os.Getenv("MIYABI_TEST_PROXY")})
	if err != nil {
		t.Fatal(err)
	}
	s := NewCaribbeancom(proxy)
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
			t.Logf("%s: release %q dur %d actors %d tags %d", code, m.Detail.ReleaseDate, m.Detail.Duration, len(m.Detail.Actors), len(m.Detail.Tags))
		})
	}
}
