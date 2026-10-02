package providers

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/jpeg"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
)

const fc2ProductPage = `<meta property="og:url" content="https://adult.contents.fc2.com/article/1234567/">
<meta property="og:image" content="https://storage200000.contents.fc2.com/original.jpeg">
<div class="items_article_MainitemThumb"><img src="//contents-thumbnail2.fc2.com/w276/cover.jpeg"><p class="items_article_info">01:24:30</p></div>
<div class="items_article_headerInfo"><h3>Official title</h3></div>
<div class="items_article_softDevice"><h3>Devices</h3><ul><li>PC</li></ul></div>
<div class="items_article_softDevice"><p>販売日 : 2026/03/22</p></div>
<div class="items_article_softDevice"><p>商品ID : FC2 PPV 1234567</p></div>
<section class="items_article_SampleImages">
<a href="//contents-thumbnail2.fc2.com/w1280/preview.jpeg"><img src="//contents-thumbnail2.fc2.com/w160/preview.jpeg"></a>
<a href="https://contents-thumbnail2.fc2.com/w1280/preview.jpeg">Duplicate</a>
<a href="https://foreign.example/unrelated.jpg">Foreign</a>
<a href="javascript:void(0)">Not an image</a>
</section>`

func TestFC2UsesDeclaredOriginalAndReadsSiblingProductDetails(t *testing.T) {
	m, err := parseFC2(fc2ProductPage, "https://adult.contents.fc2.com/", "1234567")
	if err != nil {
		t.Fatal(err)
	}
	if m.Detail.ReleaseDate != "2026-03-22" || m.Detail.Duration != 84 || m.Detail.OriginTitle != "Official title" {
		t.Fatalf("product fields missing: %+v", m.Detail)
	}
	if m.Detail.Cover != "https://storage200000.contents.fc2.com/original.jpeg" || m.Detail.Thumbnail != "https://contents-thumbnail2.fc2.com/w276/cover.jpeg" ||
		len(m.Images) != 3 || m.Images[0].URL != m.Detail.Cover || m.Images[1].URL != m.Detail.Thumbnail || m.Images[0].Layout != domain.CoverSingle {
		t.Fatalf("official original not preferred: %+v", m)
	}
	if !m.Detail.HasPreview || len(m.Detail.PreviewImages) != 1 || m.Detail.PreviewImages[0].Original != "https://contents-thumbnail2.fc2.com/w1280/preview.jpeg" {
		t.Fatalf("preview original missing or duplicated: %+v", m.Detail.PreviewImages)
	}
	body := strings.Replace(fc2ProductPage, "https://storage200000.contents.fc2.com/original.jpeg", "https://foreign.example/cover.jpeg", 1)
	m, err = parseFC2(body, "https://adult.contents.fc2.com/", "1234567")
	if err != nil || m.Detail.Cover != m.Detail.Thumbnail || len(m.Images) != 2 {
		t.Fatalf("thumbnail fallback lost: %+v %v", m, err)
	}
}

func TestFC2RejectsUnconfirmedProductsAndInvalidCodes(t *testing.T) {
	for _, body := range []string{
		strings.Replace(fc2ProductPage, "/article/1234567/", "/article/7654321/", 1),
		strings.Replace(fc2ProductPage, "FC2 PPV 1234567", "FC2 PPV 7654321", 1),
		strings.Replace(fc2ProductPage, "https://adult.contents.fc2.com/article/1234567/", "https://foreign.example/article/1234567/", 1),
		`<div class="items_article_headerInfo"><h3>Unconfirmed title</h3></div>`,
	} {
		if _, err := parseFC2(body, "https://adult.contents.fc2.com/", "1234567"); err == nil {
			t.Fatal("unconfirmed product accepted")
		}
	}
	for _, code := range []string{"", "ABP-123", "FC2-PPV-../1234567", "FC2-1234567A"} {
		if _, err := (&FC2{}).Fetch(t.Context(), domain.MovieRef{Code: code}); !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("invalid code fetched: %s %v", code, err)
		}
	}
}

func TestFC2Live(t *testing.T) {
	codes := os.Getenv("MIYABI_TEST_FC2_CODES")
	if codes == "" {
		t.Skip("set MIYABI_TEST_FC2_CODES for live verification; images are decoded in memory")
	}
	proxy, err := netx.NewProxyManager(netx.ProxyConfig{Enabled: os.Getenv("MIYABI_TEST_PROXY") != "", URL: os.Getenv("MIYABI_TEST_PROXY")})
	if err != nil {
		t.Fatal(err)
	}
	s := NewFC2(proxy)
	defer s.Close()
	for _, code := range strings.Split(codes, ",") {
		t.Run(code, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
			defer cancel()
			m, err := s.Fetch(ctx, domain.MovieRef{Code: code})
			if err != nil {
				t.Fatal(err)
			}
			if !codeid.IsFormatEquivalent(code, m.Detail.Code) || m.Detail.ID != "" || m.Detail.ReleaseDate == "" || len(m.Images) == 0 {
				t.Fatal("missing identity or core metadata")
			}
			t.Logf("%s: release %s, duration %d, previews %d", code, m.Detail.ReleaseDate, m.Detail.Duration, len(m.Detail.PreviewImages))
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
