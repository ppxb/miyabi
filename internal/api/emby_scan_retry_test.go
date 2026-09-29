package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
)

type notificationScanLibrary struct {
	LibraryManager
	err error
}

func (s *notificationScanLibrary) StartScan(context.Context) (domain.TaskInfo, error) {
	return domain.TaskInfo{ID: 1, Type: "scan", Status: "queued"}, s.err
}

func TestManualScansRetryEmbyEvenWhenLibraryScanCannotStart(t *testing.T) {
	const path = "/api/library/scan"
	for _, unavailable := range []bool{false, true} {
		t.Run(path+map[bool]string{false: " available", true: " unavailable"}[unavailable], func(t *testing.T) {
			lib := &notificationScanLibrary{}
			if unavailable {
				lib.err = domain.E(domain.KindBusy, "扫描暂不可用", nil)
			}
			emby := &stubEmbyManager{}
			router := NewRouter(Dependencies{Library: lib, Emby: emby, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if emby.retryCalls != 1 {
				t.Fatalf("pending Emby updates were not retried: %d", emby.retryCalls)
			}
			if !unavailable && recorder.Code != http.StatusAccepted {
				t.Fatalf("scan response = %d: %s", recorder.Code, recorder.Body.String())
			}
			if unavailable && recorder.Code < 400 {
				t.Fatal("scan failure was hidden")
			}
		})
	}
}
