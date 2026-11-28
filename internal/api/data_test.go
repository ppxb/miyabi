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
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/maintenance"
)

type dataStub struct {
	info      maintenance.Info
	err       error
	reads     int
	refreshed int
	cleared   int
}

func (stub *dataStub) Info(context.Context) (maintenance.Info, error) {
	stub.reads++
	return stub.info, stub.err
}

func (stub *dataStub) RefreshInfo(context.Context) (maintenance.Info, error) {
	stub.refreshed++
	return stub.info, stub.err
}

func (stub *dataStub) ClearCache(context.Context) (maintenance.Info, error) {
	stub.cleared++
	return stub.info, stub.err
}

type logStub struct {
	meta  maintenance.LogMeta
	info  maintenance.LogInfo
	err   error
	reads int
}

func (stub *logStub) LogMeta(context.Context) (maintenance.LogMeta, error) {
	return stub.meta, stub.err
}

func (stub *logStub) Logs(context.Context, int) (maintenance.LogInfo, error) {
	stub.reads++
	return stub.info, stub.err
}

func TestDataLogsHandlerSkipsUnchangedLog(t *testing.T) {
	stub := &logStub{
		meta: maintenance.LogMeta{Path: "/app/data/miyabi.log", SizeBytes: 10, ModTime: time.Unix(1700000000, 0)},
		info: maintenance.LogInfo{Path: "/app/data/miyabi.log", Lines: []string{"line"}, SizeBytes: 10},
	}
	router := NewRouter(Dependencies{
		Access: NewAccessGateService("", ""),
		Logs:   stub,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/settings/logs", nil))
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || stub.reads != 1 || etag == "" ||
		first.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("first response: status=%d reads=%d etag=%q headers=%v", first.Code, stub.reads, etag, first.Header())
	}

	secondRequest := httptest.NewRequest(http.MethodGet, "/api/settings/logs", nil)
	secondRequest.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	router.ServeHTTP(second, secondRequest)
	if second.Code != http.StatusNotModified || stub.reads != 1 || second.Body.Len() != 0 {
		t.Fatalf("unchanged response: status=%d reads=%d body=%q", second.Code, stub.reads, second.Body)
	}

	stub.meta.SizeBytes = 11
	thirdRequest := httptest.NewRequest(http.MethodGet, "/api/settings/logs", nil)
	thirdRequest.Header.Set("If-None-Match", etag)
	third := httptest.NewRecorder()
	router.ServeHTTP(third, thirdRequest)
	if third.Code != http.StatusOK || stub.reads != 2 {
		t.Fatalf("changed response: status=%d reads=%d", third.Code, stub.reads)
	}
}

func TestDataEndpointsReturnUncachedStatsAndCleanupErrors(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		method  string
		path    string
		refresh bool
		err     error
		status  int
	}{
		{name: "statistics", method: http.MethodGet, path: "/api/settings/system", status: http.StatusOK},
		{name: "statistics refresh", method: http.MethodGet, path: "/api/settings/system?refresh=1", refresh: true, status: http.StatusOK},
		{name: "cleanup", method: http.MethodDelete, path: "/api/settings/cache", status: http.StatusOK},
		{name: "busy", method: http.MethodDelete, path: "/api/settings/cache", err: maintenance.ErrCacheBusy, status: http.StatusConflict},
		{name: "cleanup failure", method: http.MethodDelete, path: "/api/settings/cache", err: errors.New("cleanup failed"), status: http.StatusInternalServerError},
		{name: "statistics failure", method: http.MethodGet, path: "/api/settings/system", err: errors.New("statistics failed"), status: http.StatusInternalServerError},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			stub := &dataStub{err: scenario.err, info: maintenance.Info{
				DataDirectory: "/app/data", DatabaseSizeBytes: 1024,
				Cache: mediaimage.CacheStats{SizeBytes: 4096, EntryCount: 2},
			}}
			router := NewRouter(Dependencies{Access: NewAccessGateService("", ""), Maintenance: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(scenario.method, scenario.path, nil))
			if response.Code != scenario.status || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body)
			}
			switch {
			case scenario.method == http.MethodDelete:
				if stub.reads != 0 || stub.refreshed != 0 || stub.cleared != 1 {
					t.Fatalf("unexpected data operations: reads=%d refreshed=%d cleared=%d", stub.reads, stub.refreshed, stub.cleared)
				}
			case scenario.refresh:
				if stub.reads != 0 || stub.refreshed != 1 || stub.cleared != 0 {
					t.Fatalf("unexpected data operations: reads=%d refreshed=%d cleared=%d", stub.reads, stub.refreshed, stub.cleared)
				}
			default:
				if stub.reads != 1 || stub.refreshed != 0 || stub.cleared != 0 {
					t.Fatalf("unexpected data operations: reads=%d refreshed=%d cleared=%d", stub.reads, stub.refreshed, stub.cleared)
				}
			}
			if scenario.err != nil {
				var failure struct {
					Error string `json:"error"`
				}
				wantMsg := domain.PublicMessage(scenario.err)
				if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil || failure.Error != wantMsg {
					t.Fatalf("error response=%s %v", response.Body, err)
				}
			} else {
				var info maintenance.Info
				if err := json.Unmarshal(response.Body.Bytes(), &info); err != nil || info != stub.info {
					t.Fatalf("data response=%s %v", response.Body, err)
				}
			}
		})
	}
}
