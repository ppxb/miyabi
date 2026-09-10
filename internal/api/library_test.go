package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ppxb/miyabi/internal/service"
)

type libraryWatchStub struct {
	LibraryManager
	id  int
	err error
}

func (stub *libraryWatchStub) MarkWatched(_ context.Context, id int) error {
	stub.id = id
	return stub.err
}

func TestLibraryWatchedEndpointValidatesIDsAndReturnsSavedState(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		id     string
		err    error
		status int
		called bool
	}{
		{name: "saved", id: "42", status: http.StatusOK, called: true},
		{name: "zero", id: "0", status: http.StatusBadRequest},
		{name: "negative", id: "-1", status: http.StatusBadRequest},
		{name: "not numeric", id: "movie", status: http.StatusBadRequest},
		{name: "overflow", id: "99999999999999999999", status: http.StatusBadRequest},
		{name: "missing movie", id: "42", err: fs.ErrNotExist, status: http.StatusNotFound, called: true},
		{name: "unmounted", id: "42", err: service.ErrMediaDirectoryRequired, status: http.StatusBadRequest, called: true},
		{name: "write failed", id: "42", err: errors.New("write failed"), status: http.StatusInternalServerError, called: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			stub := &libraryWatchStub{err: scenario.err}
			router := NewRouter(Dependencies{Library: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/library/movies/"+scenario.id+"/watched", nil))
			if response.Code != scenario.status || (stub.id != 0) != scenario.called {
				t.Fatalf("status=%d called=%d body=%s", response.Code, stub.id, response.Body)
			}
			if scenario.status == http.StatusOK {
				var state struct {
					ID      int  `json:"id"`
					Watched bool `json:"watched"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil || state.ID != 42 || !state.Watched {
					t.Fatalf("incorrect watch response: %s, %v", response.Body, err)
				}
			}
		})
	}
}
