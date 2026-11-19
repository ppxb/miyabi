package actors

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"golang.org/x/net/html"
)

func TestParseMeasurements(t *testing.T) {
	m, cup := parseMeasurements("B:84（F） / W:58 / H:88")
	if m != "B:84 / W:58 / H:88" || cup != "F" {
		t.Fatalf("parseMeasurements = %q %q", m, cup)
	}
}

func TestJapaneseDate(t *testing.T) {
	if got := japaneseDate("1993年8月16日（33歳）"); got != "1993-08-16" {
		t.Fatalf("japaneseDate = %q", got)
	}
}

func TestAvLeagueImagesSkipsPlaceholders(t *testing.T) {
	doc, _ := html.Parse(bytes.NewBufferString(`<html><body>
<div class="i-pic-box"><div><img src="data:image/gif;base64,xxx" data-layzr="https://www.av-league.com/image/act/1/1-0.webp"></div></div>
<meta property="og:image" content="https://www.av-league.com/image/act/1/1-0.webp">
</body></html>`))
	images := avLeagueImages(doc, "https://www.av-league.com/")
	if len(images) != 1 || images[0] != "https://www.av-league.com/image/act/1/1-0.webp" {
		t.Fatalf("images = %v", images)
	}
}

func TestLoadImageCachesOnDisk(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8\xff\xe0fakejpeg"))
	}))
	defer srv.Close()

	svc := &Service{dataDir: t.TempDir(), imageHTTP: srv.Client()}
	src := imageSource{provider: "av-league", ref: srv.URL}
	first, err := svc.loadImage(t.Context(), src)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.loadImage(t.Context(), src)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || len(first) == 0 {
		t.Fatalf("cached bytes mismatch")
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("expected 1 remote fetch, got %d", hits)
	}
}
