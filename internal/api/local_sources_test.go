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
	"github.com/ppxb/miyabi/internal/localsource"
)

type localSourceStub struct {
	LocalSourceManager
	sources []localsource.Source
	added   []string
	updated []string
	update  localsource.Update
	removed string
	err     error
}

func (stub *localSourceStub) List(context.Context) ([]localsource.Source, error) {
	return stub.sources, stub.err
}

func (stub *localSourceStub) Add(_ context.Context, name, path string) (localsource.Source, error) {
	stub.added = []string{name, path}
	if stub.err != nil {
		return localsource.Source{}, stub.err
	}
	return localsource.Source{ID: "src-1", Name: name, Path: path, Enabled: true}, nil
}

func (stub *localSourceStub) Update(_ context.Context, id string, update localsource.Update) (localsource.Source, error) {
	stub.updated, stub.update = []string{id}, update
	if stub.err != nil {
		return localsource.Source{}, stub.err
	}
	return localsource.Source{ID: id, Name: "下载目录", Path: "/media/inbox"}, nil
}

func (stub *localSourceStub) Remove(_ context.Context, id string) error {
	stub.removed = id
	return stub.err
}

func localSourceTestRouter(source LocalSourceManager, library LibraryManager) http.Handler {
	return NewRouter(Dependencies{
		Access: NewAccessGateService("", ""), Library: library, LocalSources: source,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func TestLocalSourceSettingsManageConfiguredDirectories(t *testing.T) {
	for _, scenario := range []struct {
		name, method, path, body string
		status                   int
	}{
		{name: "list", method: http.MethodGet, path: "/api/settings/local-sources", status: http.StatusOK},
		{name: "create", method: http.MethodPost, path: "/api/settings/local-sources", body: `{"name":"下载目录","path":"/media/inbox"}`, status: http.StatusCreated},
		{name: "create without a path", method: http.MethodPost, path: "/api/settings/local-sources", body: `{"name":"下载目录"}`, status: http.StatusBadRequest},
		{name: "disable", method: http.MethodPatch, path: "/api/settings/local-sources/src-1", body: `{"enabled":false}`, status: http.StatusOK},
		{name: "remove", method: http.MethodDelete, path: "/api/settings/local-sources/src-1", status: http.StatusOK},
		{name: "remove without an id", method: http.MethodDelete, path: "/api/settings/local-sources/", status: http.StatusNotFound},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			stub := &localSourceStub{sources: []localsource.Source{{ID: "src-1", Name: "下载目录", Path: "/media/inbox", Enabled: true}}}
			request := httptest.NewRequest(scenario.method, scenario.path, strings.NewReader(scenario.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			localSourceTestRouter(stub, nil).ServeHTTP(response, request)
			if response.Code != scenario.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, scenario.status, response.Body)
			}
			switch scenario.name {
			case "list":
				if !strings.Contains(response.Body.String(), `"path":"/media/inbox"`) {
					t.Fatalf("list body = %s", response.Body)
				}
			case "create":
				if len(stub.added) != 2 || stub.added[1] != "/media/inbox" || !strings.Contains(response.Body.String(), `"id":"src-1"`) {
					t.Fatalf("created source = %+v: %s", stub.added, response.Body)
				}
			case "disable":
				if len(stub.updated) != 1 || stub.updated[0] != "src-1" || stub.update.Enabled == nil || *stub.update.Enabled {
					t.Fatalf("update = %+v %+v", stub.updated, stub.update)
				}
			case "remove":
				if stub.removed != "src-1" {
					t.Fatalf("removed = %q", stub.removed)
				}
			}
		})
	}
}

type localScanLibraryStub struct {
	LibraryManager
	sourceID string
	err      error
}

func (stub *localScanLibraryStub) StartLocalScan(_ context.Context, id string) (domain.TaskInfo, error) {
	stub.sourceID = id
	if stub.err != nil {
		return domain.TaskInfo{}, stub.err
	}
	return domain.TaskInfo{
		ID: 7, Type: "scan", Status: "queued",
		Source: domain.LibrarySource{AccountID: domain.LocalAccountID, Directory: domain.LibraryDirectory{ID: id, Name: "下载目录", Path: "/media/inbox"}},
	}, nil
}

func TestLibraryLocalScanQueuesTheRequestedSource(t *testing.T) {
	for _, scenario := range []struct {
		name, body string
		failure    error
		status     int
		called     bool
	}{
		{name: "configured source", body: `{"source_id":"src-1"}`, status: http.StatusAccepted, called: true},
		{name: "unknown source", body: `{"source_id":"src-1"}`, failure: domain.E(domain.KindNotFound, "本地媒体目录不存在或已停用", nil), status: http.StatusNotFound, called: true},
		{name: "missing source", body: `{}`, status: http.StatusBadRequest},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			library := &localScanLibraryStub{err: scenario.failure}
			request := httptest.NewRequest(http.MethodPost, "/api/library/local-scan", strings.NewReader(scenario.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			localSourceTestRouter(nil, library).ServeHTTP(response, request)
			if response.Code != scenario.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, scenario.status, response.Body)
			}
			if !scenario.called {
				if library.sourceID != "" {
					t.Fatalf("invalid request scanned %q", library.sourceID)
				}
				return
			}
			if library.sourceID != "src-1" {
				t.Fatalf("scanned source = %q", library.sourceID)
			}
			if scenario.failure == nil && !strings.Contains(response.Body.String(), `"id":"src-1"`) {
				t.Fatalf("task body = %s", response.Body)
			}
		})
	}
}
