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

type libraryPageStub struct {
	LibraryManager
	page, limit int
}

func (stub *libraryPageStub) Movies(_ context.Context, page, limit int) (service.LibraryPage, error) {
	stub.page, stub.limit = page, limit
	return service.LibraryPage{Page: page, Movies: []service.LibraryMovie{}}, nil
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
			router := NewRouter(Dependencies{Library: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/library/movies"+scenario.query, nil))
			if response.Code != http.StatusOK || stub.page != scenario.page || stub.limit != 20 {
				t.Fatalf("library pagination: status=%d page=%d limit=%d body=%s", response.Code, stub.page, stub.limit, response.Body)
			}
		})
	}
}

func (stub *libraryWatchStub) MarkWatched(_ context.Context, id int) (service.WatchSession, error) {
	stub.id = id
	return service.WatchSession{ID: 7, SessionID: "session", FileID: "video", Position: 60, Duration: 600}, stub.err
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
					ID      int                  `json:"id"`
					Watched bool                 `json:"watched"`
					History service.WatchSession `json:"history"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil || state.ID != 42 || !state.Watched || state.History.Position != 60 {
					t.Fatalf("incorrect watch response: %s, %v", response.Body, err)
				}
			}
		})
	}
}
