package providers

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/metadata"
)

func TestAVBasePageExtractsWorkCastsAndOriginalProductImages(t *testing.T) {
	page, err := parseAVBasePage([]byte(`<html><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"work":{
"id":42,"work_id":"SSIS-001","title":"Work title","min_date":"2021-02-18","genres":[{"id":7,"name":"Tag"}],
"casts":[{"actor":{"id":100,"name":"Actor","image_url":"https://www.avbase.net/avatar.jpg"}},{"actor":{"id":101,"name":"Actor"}}],
"products":[{"source":"fanza","product_id":"ssis00001","title":"Product title","image_url":"https://pics.dmm.co.jp/large.jpg","thumbnail_url":"https://pics.dmm.co.jp/small.jpg","maker":{"id":2,"name":"Studio"},"iteminfo":{"description":"Synopsis","volume":"150分"},"sample_image_urls":[{"s":"https://pics.dmm.co.jp/sample-small.jpg","l":"https://pics.dmm.co.jp/sample-large.jpg"}]}]
}}}}</script></html>`))
	if err != nil || page.Work == nil {
		t.Fatalf("page: %+v %v", page, err)
	}
	result, err := avbaseMetadata(*page.Work)
	if err != nil {
		t.Fatal(err)
	}
	m := result.Detail
	if m.Code != "SSIS-001" || m.Title != "Work title" || m.Duration != 150 || m.Summary != "Synopsis" || m.Maker.ID != "2" || len(m.Actors) != 2 || m.Actors[0].ID == m.Actors[1].ID {
		t.Fatalf("metadata: %+v", m)
	}
	if len(result.Images) != 2 || result.Images[0].URL != "https://pics.dmm.co.jp/large.jpg" || result.Images[1].URL != "https://pics.dmm.co.jp/sample-large.jpg" {
		t.Fatalf("selected thumbnails: %+v", result.Images)
	}
	if m.Sources[0].Provider != "avbase" || m.Sources[1].ID != "ssis00001" {
		t.Fatalf("lost source identities: %+v", m.Sources)
	}
}

func TestAVBaseMatchingRejectsAmbiguousPrefixesAndUnrelatedNumbers(t *testing.T) {
	if _, err := matchAVBase([]avbaseWork{{Prefix: "fanza", Code: "ABC-001"}, {Prefix: "mgstage", Code: "ABC-001"}}, "ABC-001"); err == nil {
		t.Fatal("accepted ambiguous work prefixes")
	}
	if _, err := matchAVBase([]avbaseWork{{Code: "ABC-002"}}, "ABC-001"); err != metadata.ErrNotFound {
		t.Fatalf("unrelated match: %v", err)
	}
	work, err := matchAVBase([]avbaseWork{{Code: "ABC-0001"}, {Code: "ABC-001"}}, "ABC-001")
	if err != nil || work.Code != "ABC-001" {
		t.Fatalf("exact number did not win: %+v %v", work, err)
	}
	if _, err := parseAVBasePage([]byte(`<title>Just a moment...</title>`)); err == nil {
		t.Fatal("challenge accepted as metadata")
	}
}

func TestAVBaseLive(t *testing.T) {
	code := os.Getenv("MIYABI_TEST_AVBASE_CODE")
	if code == "" {
		t.Skip("set MIYABI_TEST_AVBASE_CODE for live AVBase verification")
	}
	s, err := NewAVBase(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	result, err := s.Fetch(ctx, code)
	if err != nil {
		t.Fatal(err)
	}
	if result.Detail.Code == "" || len(result.Detail.Actors) == 0 || len(result.Images) == 0 {
		t.Fatal("live response lacks metadata or images")
	}
}
