package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ppxb/miyabi/internal/library"
)

type libraryPageStub struct {
	LibraryManager
	page, limit int
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
			router := NewRouter(Dependencies{Library: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/library/movies"+scenario.query, nil))
			if response.Code != http.StatusOK || stub.page != scenario.page || stub.limit != 20 {
				t.Fatalf("library pagination: status=%d page=%d limit=%d body=%s", response.Code, stub.page, stub.limit, response.Body)
			}
		})
	}
}
