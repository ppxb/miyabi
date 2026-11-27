package api

import (
	"bytes"
	"context"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/disintegration/imaging"
	"github.com/ppxb/miyabi/internal/domain"
	mediaimage "github.com/ppxb/miyabi/internal/image"
)

type imageCatalogueStub struct {
	CatalogueManager
	calls int
	body  []byte
}

func (stub *imageCatalogueStub) Media(context.Context, string) (domain.Media, error) {
	stub.calls++
	return domain.Media{ContentType: "image/png", Body: stub.body}, nil
}

func TestImageHandlerCachesThumbnails(t *testing.T) {
	cache, err := mediaimage.NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, imaging.New(200, 100, color.White)); err != nil {
		t.Fatal(err)
	}
	stub := &imageCatalogueStub{body: buffer.Bytes()}
	router := NewRouter(Dependencies{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Access:    NewAccessGateService("", ""),
		Catalogue: stub,
		Images:    cache,
	})
	target := "/api/image?url=" + url.QueryEscape("https://cdn.example/cover.png") + "&w=80"
	for attempt := 1; attempt <= 2; attempt++ {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/jpeg" {
			t.Fatalf("attempt %d: status=%d content-type=%q", attempt, response.Code, response.Header().Get("Content-Type"))
		}
		decoded, err := imaging.Decode(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got := decoded.Bounds().Dx(); got != 80 {
			t.Fatalf("attempt %d: width = %d, want 80", attempt, got)
		}
	}
	if stub.calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", stub.calls)
	}
}
