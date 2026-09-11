package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/service"
)

type dataStub struct {
	info    service.DataInfo
	err     error
	reads   int
	cleared int
}

func (stub *dataStub) Info(context.Context) (service.DataInfo, error) {
	stub.reads++
	return stub.info, stub.err
}

func (stub *dataStub) ClearCache(context.Context) (service.DataInfo, error) {
	stub.cleared++
	return stub.info, stub.err
}

func TestDataEndpointsReturnUncachedStatsAndCleanupErrors(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		method string
		path   string
		err    error
		status int
	}{
		{name: "statistics", method: http.MethodGet, path: "/api/settings/system", status: http.StatusOK},
		{name: "cleanup", method: http.MethodDelete, path: "/api/settings/cache", status: http.StatusOK},
		{name: "busy", method: http.MethodDelete, path: "/api/settings/cache", err: service.ErrCacheBusy, status: http.StatusConflict},
		{name: "cleanup failure", method: http.MethodDelete, path: "/api/settings/cache", err: errors.New("cleanup failed"), status: http.StatusInternalServerError},
		{name: "statistics failure", method: http.MethodGet, path: "/api/settings/system", err: errors.New("statistics failed"), status: http.StatusInternalServerError},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			stub := &dataStub{err: scenario.err, info: service.DataInfo{
				DataDirectory: "/app/data", DatabaseSizeBytes: 1024,
				Cache: mediaimage.CacheStats{SizeBytes: 4096, EntryCount: 2},
			}}
			router := NewRouter(Dependencies{Data: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(scenario.method, scenario.path, nil))
			if response.Code != scenario.status || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body)
			}
			if (scenario.method == http.MethodGet && (stub.reads != 1 || stub.cleared != 0)) ||
				(scenario.method == http.MethodDelete && (stub.reads != 0 || stub.cleared != 1)) {
				t.Fatalf("unexpected data operations: reads=%d cleared=%d", stub.reads, stub.cleared)
			}
			if scenario.err != nil {
				var failure struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil || failure.Error != scenario.err.Error() {
					t.Fatalf("error response=%s %v", response.Body, err)
				}
			} else {
				var info service.DataInfo
				if err := json.Unmarshal(response.Body.Bytes(), &info); err != nil || info != stub.info {
					t.Fatalf("data response=%s %v", response.Body, err)
				}
			}
		})
	}
}
