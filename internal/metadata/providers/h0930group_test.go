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

const h0930Page = `<html><head>
<meta name="movieid" content="ki220913">
<script type="application/ld+json">{"@type":"WebSite","name":"site"}</script>
<script type="application/ld+json">{"@type":"VideoObject","name":"サンプル","image":"https://smovie.c0930.com/x/cover.jpg","description":"あらすじ","actor":{"name":"女優A"},"video":{"duration":"PT1H5M","provider":"人妻斬り"},"releasedEvent":{"startDate":"2022-09-13"},"aggregateRating":{"ratingValue":"4.5"}}</script>
</head><body>
<div id="moviePlay"><div class="moviePlay_title"><h1><span>サンプル見出し</span></h1></div></div>
<div id="movieInfo"><section><dl>
<dt>公開日</dt><dd>2022-09-13</dd>
<dt>動画</dt><dd>PT1H5M</dd>
<dt>プレイ内容</dt><dd>企画A&nbsp;企画B</dd>
</dl></section></div>
<video id="movieContent" poster="https://smovie.c0930.com/x/poster.jpg"></video>
<div id="movieGallery"><script type="text/javascript">var g=[href="https://smovie.c0930.com/x/1.jpg", href="https://smovie.c0930.com/member/x.jpg"];</script></div>
</body></html>`

func TestH0930ParsesDetail(t *testing.T) {
	m, err := parseH0930(h0930Page, "https://www.c0930.com/", "KI-220913", "ki220913", "c0930.com", "c0930", "人妻斬り")
	if err != nil {
		t.Fatal(err)
	}
	if m.Detail.Title != "サンプル見出し" || m.Detail.Plot != "あらすじ" || m.Detail.Zone != domain.ZoneUncensored {
		t.Fatalf("title/plot/zone wrong: %+v", m.Detail)
	}
	if m.Detail.ReleaseDate != "2022-09-13" || m.Detail.Duration != 65 {
		t.Fatalf("release/duration wrong: %+v", m.Detail)
	}
	if len(m.Detail.Actors) != 1 || m.Detail.Actors[0].Name != "女優A" {
		t.Fatalf("actors wrong: %+v", m.Detail.Actors)
	}
	if m.Detail.Maker == nil || m.Detail.Maker.Name != "人妻斬り" {
		t.Fatalf("maker wrong: %+v", m.Detail.Maker)
	}
	if len(m.Detail.Tags) != 2 {
		t.Fatalf("tags wrong: %+v", m.Detail.Tags)
	}
	if m.Detail.Rating != 4.5 || m.Detail.RatingMax != 5 {
		t.Fatalf("rating wrong: %+v", m.Detail)
	}
	if m.Detail.Cover != "https://smovie.c0930.com/x/poster.jpg" {
		t.Fatalf("cover wrong: %q", m.Detail.Cover)
	}
	if len(m.Detail.PreviewImages) != 1 {
		t.Fatalf("preview wrong (member must drop): %+v", m.Detail.PreviewImages)
	}
}

func TestH0930RejectsSoftPage(t *testing.T) {
	// Title via JSON-LD but no player/info container and no id -> reject.
	body := `<html><head><script type="application/ld+json">{"@type":"VideoObject","name":"t"}</script></head><body></body></html>`
	if _, err := parseH0930(body, "https://www.c0930.com/", "KI-220913", "ki220913", "c0930.com", "c0930", "人妻斬り"); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("soft page accepted: %v", err)
	}
}

func TestH0930GroupSupports(t *testing.T) {
	c := NewC0930(nil)
	h := NewH0930(nil)
	f := NewH4610(nil)
	for _, code := range []string{"KI-220913", "HITOZUMA-1391"} {
		if !c.Supports(code) {
			t.Fatalf("c0930 should support %q", code)
		}
	}
	for _, code := range []string{"ORI-1643", "ORIJUKU-823"} {
		if !h.Supports(code) {
			t.Fatalf("h0930 should support %q", code)
		}
	}
	for _, code := range []string{"TK-0047", "PLA-0051"} {
		if !f.Supports(code) {
			t.Fatalf("h4610 should support %q", code)
		}
	}
	for _, code := range []string{"SSIS-033", "ABP-123"} {
		if c.Supports(code) || h.Supports(code) || f.Supports(code) {
			t.Fatalf("group should not support %q", code)
		}
	}
	if _, err := c.Fetch(t.Context(), domain.MovieRef{Code: "SSIS-033"}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("unsupported c0930 fetched: %v", err)
	}
}

func TestC0930Live(t *testing.T) {
	codes := os.Getenv("MIYABI_TEST_H0930_CODES")
	if codes == "" {
		t.Skip("set MIYABI_TEST_H0930_CODES for live verification")
	}
	proxy, err := netx.NewProxyManager(netx.ProxyConfig{Enabled: os.Getenv("MIYABI_TEST_PROXY") != "", URL: os.Getenv("MIYABI_TEST_PROXY")})
	if err != nil {
		t.Fatal(err)
	}
	s := NewC0930(proxy)
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
