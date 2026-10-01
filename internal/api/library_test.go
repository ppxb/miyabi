package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/library"
)

type libraryPageStub struct {
	LibraryManager
	page, limit int
}

type libraryRebuildStub struct {
	LibraryManager
	err error
}

func (stub *libraryRebuildStub) StartRebuild(context.Context) (domain.TaskInfo, error) {
	return domain.TaskInfo{ID: 42, Type: "scan", Status: "queued", Rebuild: true}, stub.err
}

func TestLibraryRebuildReturnsTaskAndPropagatesSourceErrors(t *testing.T) {
	for _, failure := range []error{nil, domain.E(domain.KindConflict, "媒体目录未挂载", nil)} {
		router := NewRouter(Dependencies{Access: NewAccessGateService("", ""), Library: &libraryRebuildStub{err: failure}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/library/rebuild", nil))
		want := http.StatusAccepted
		if failure != nil {
			want = http.StatusConflict
		}
		if response.Code != want {
			t.Fatalf("rebuild status=%d body=%s", response.Code, response.Body)
		}
	}
}

type libraryPreviewStub struct {
	LibraryManager
	candidate domain.ImageCandidate
}

func (stub *libraryPreviewStub) Preview(_ context.Context, id, index int) (domain.ImageCandidate, error) {
	if id != 42 || index != 0 {
		return domain.ImageCandidate{}, domain.E(domain.KindNotFound, "预览图不存在", nil)
	}
	return stub.candidate, nil
}

type metadataImageStub struct {
	MetadataManager
	received domain.ImageCandidate
}

func (stub *metadataImageStub) Image(_ context.Context, candidate domain.ImageCandidate) (domain.Media, error) {
	stub.received = candidate
	return domain.Media{ContentType: "image/jpeg", Body: []byte("preview bytes")}, nil
}

func TestLibraryPreviewRoutesSavedSourceAndRejectsInvalidTargets(t *testing.T) {
	for _, tc := range []struct {
		path, provider string
		status         int
	}{
		{"/42/previews/0?v=123", "fanza", http.StatusOK},
		{"/42/previews/0", "fc2", http.StatusOK},
		{"/42/previews/0", "javdb", http.StatusOK},
		{"/42/previews/1", "fanza", http.StatusNotFound},
		{"/43/previews/0", "fanza", http.StatusNotFound},
		{"/0/previews/0", "fanza", http.StatusBadRequest},
		{"/42/previews/-1", "fanza", http.StatusBadRequest},
		{"/42/previews/invalid", "fanza", http.StatusBadRequest},
	} {
		t.Run(tc.provider+tc.path, func(t *testing.T) {
			candidate := domain.ImageCandidate{Provider: tc.provider, URL: "https://image.example/preview.jpg", Role: "preview"}
			metadata := &metadataImageStub{}
			router := NewRouter(Dependencies{
				Access: NewAccessGateService("", ""), Library: &libraryPreviewStub{candidate: candidate}, Metadata: metadata,
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/library/movies"+tc.path, nil))
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.status, response.Body)
			}
			if tc.status == http.StatusOK {
				if metadata.received != candidate || response.Body.String() != "preview bytes" ||
					response.Header().Get("Content-Type") != "image/jpeg" || response.Header().Get("Cache-Control") != "private, max-age=3600" {
					t.Fatalf("preview changed source or response: %+v, %s", metadata.received, response.Body)
				}
			} else if metadata.received != (domain.ImageCandidate{}) {
				t.Fatalf("invalid target fetched an image: %+v", metadata.received)
			}
		})
	}
}

func (stub *libraryPageStub) Movies(_ context.Context, page, limit int) (library.Page, error) {
	stub.page, stub.limit = page, limit
	return library.Page{Page: page, Movies: []library.Movie{}}, nil
}

func TestLibraryMoviesDefaultToTwentyPerPage(t *testing.T) {
	for _, scenario := range []struct {
		query string
		page  int
	}{
		{query: "", page: 1},
		{query: "?page=2", page: 2},
	} {
		t.Run(scenario.query, func(t *testing.T) {
			stub := &libraryPageStub{}
			router := NewRouter(Dependencies{Access: NewAccessGateService("", ""), Library: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/library/movies"+scenario.query, nil))
			if response.Code != http.StatusOK || stub.page != scenario.page || stub.limit != 20 {
				t.Fatalf("library pagination: status=%d page=%d limit=%d body=%s", response.Code, stub.page, stub.limit, response.Body)
			}
		})
	}
}
