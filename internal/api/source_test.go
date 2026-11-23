package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ppxb/miyabi/internal/source"
)

type sourceStoreStub struct {
	saved []source.Source
}

func (stub *sourceStoreStub) List(context.Context) ([]source.Source, error) {
	return append([]source.Source(nil), stub.saved...), nil
}

func (stub *sourceStoreStub) Replace(_ context.Context, sources []source.Source) ([]source.Source, error) {
	for i := range sources {
		if sources[i].ID == "" {
			sources[i].ID = "new"
		}
	}
	stub.saved = sources
	return sources, nil
}

type librarySyncStub struct {
	LibraryManager
	syncs int
}

func (stub *librarySyncStub) StartIncrementalSync(context.Context) (int, error) {
	stub.syncs++
	return 1, nil
}

func TestSourceUpdateQueuesScansOnlyWhenScanTargetsChange(t *testing.T) {
	store := &sourceStoreStub{saved: []source.Source{
		{ID: "a", Kind: "local", Name: "A", Enabled: true, Config: map[string]string{"path": "/x"}},
	}}
	library := &librarySyncStub{}
	router := NewRouter(Dependencies{Access: NewAccessGateService("", ""), Sources: store, Library: library, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	save := func(sources []source.Source, wantSyncs int) {
		t.Helper()
		body, err := json.Marshal(sources)
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/settings/sources", bytes.NewReader(body)))
		if response.Code != http.StatusOK {
			t.Fatalf("response = %d %s", response.Code, response.Body)
		}
		if library.syncs != wantSyncs {
			t.Fatalf("syncs = %d, want %d after saving %+v", library.syncs, wantSyncs, sources)
		}
	}
	local := func(name string, enabled bool, path string) source.Source {
		return source.Source{ID: "a", Kind: "local", Name: name, Enabled: enabled, Config: map[string]string{"path": path}}
	}

	save([]source.Source{local("Renamed", true, "/x")}, 0)
	save([]source.Source{local("Renamed", false, "/x")}, 1)
	save([]source.Source{local("Renamed", false, "/y")}, 2)
	save([]source.Source{local("Renamed", false, "/y"), {Kind: "local", Enabled: true, Config: map[string]string{"path": "/z"}}}, 3)
}
