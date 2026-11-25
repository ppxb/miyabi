package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ppxb/miyabi/internal/catalogue"
)

type magnetsStub struct {
	CatalogueManager
	code, javdbID string
	calls         int
}

func (s *magnetsStub) MagnetsByCode(_ context.Context, code, javdbID string) ([]catalogue.Magnet, error) {
	s.code, s.javdbID = code, javdbID
	s.calls++
	return []catalogue.Magnet{}, nil
}

func TestDiscoverMagnetsLooksUpByCodeOnly(t *testing.T) {
	for _, tc := range []struct {
		path          string
		status        int
		code, javdbID string
	}{
		{"/ZY5eq/magnets?code=ssis-001&javdb_id=ZY5eq", http.StatusOK, "SSIS-001", "ZY5eq"},
		{"/SSIS-001/magnets", http.StatusOK, "SSIS-001", ""},
		// Without a valid code there is no fallback lookup by movie id.
		{"/movie/magnets", http.StatusBadRequest, "", ""},
	} {
		stub := &magnetsStub{}
		router := NewRouter(Dependencies{Access: NewAccessGateService("", ""), Catalogue: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/discover/movies"+tc.path, nil))
		if response.Code != tc.status {
			t.Fatalf("%s: status=%d want %d: %s", tc.path, response.Code, tc.status, response.Body)
		}
		wantCalls := map[bool]int{true: 1}[tc.status == http.StatusOK]
		if stub.calls != wantCalls || stub.code != tc.code || stub.javdbID != tc.javdbID {
			t.Fatalf("%s: calls=%d code=%q javdb_id=%q", tc.path, stub.calls, stub.code, stub.javdbID)
		}
	}
}
