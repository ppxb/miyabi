package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
)

type stubLocalScanLibrary struct {
	LibraryManager
	requested string
	called    bool
	err       error
}

func (s *stubLocalScanLibrary) StartLocalScan(_ context.Context, path string) (domain.TaskInfo, error) {
	s.called, s.requested = true, path
	return domain.TaskInfo{ID: 42, Type: "scan", Status: "queued"}, s.err
}

func TestLibraryLocalScanHandler(t *testing.T) {
	for _, test := range []struct {
		name, body, path string
		err              error
		status           int
		called           bool
	}{
		{name: "current directory", body: "{}", status: 202, called: true},
		{name: "empty body", status: 202, called: true},
		{name: "explicit path", body: `{"path":"/emby"}`, path: "/emby", status: 202, called: true},
		{name: "invalid JSON", body: "{invalid-json", status: 400},
		{name: "outside configured directory", body: `{"path":"/outside"}`, path: "/outside", err: domain.E(domain.KindInvalid, "只能扫描当前配置的 Emby 本地目录", nil), status: 400, called: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			lib := &stubLocalScanLibrary{err: test.err}
			router := NewRouter(Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Library: lib})
			req := httptest.NewRequest(http.MethodPost, "/api/library/scan/local", strings.NewReader(test.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != test.status || lib.called != test.called || lib.requested != test.path {
				t.Fatalf("status=%d called=%v path=%q body=%s", w.Code, lib.called, lib.requested, w.Body.String())
			}
			if w.Code == http.StatusAccepted {
				var info domain.TaskInfo
				if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil || info.ID != 42 || info.Status != "queued" {
					t.Fatalf("unexpected task response: %s (%v)", w.Body.String(), err)
				}
			}
		})
	}
}
