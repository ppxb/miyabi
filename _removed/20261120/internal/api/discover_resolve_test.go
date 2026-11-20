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

type discoverResolverStub struct {
	CatalogueManager
	code string
	err  error
}

func (s *discoverResolverStub) ResolveMovieID(_ context.Context, code string) (string, error) {
	s.code = code
	return "confirmed-id", s.err
}

func TestDiscoverResolveMovie(t *testing.T) {
	for _, tc := range []struct {
		name, query, code string
		err               error
		status            int
	}{
		{name: "code lookup bypasses detail ID route", query: "?code=abp123", code: "ABP-123", status: http.StatusOK},
		{name: "missing code", status: http.StatusBadRequest},
		{name: "blank code", query: "?code=%20%20", status: http.StatusBadRequest},
		{name: "oversized code", query: "?code=" + strings.Repeat("a", 201), status: http.StatusBadRequest},
		{name: "ambiguous movie", query: "?code=ABP-123", code: "ABP-123", err: domain.E(domain.KindConflict, "无法唯一匹配影片", nil), status: http.StatusConflict},
		{name: "upstream failure", query: "?code=ABP-123", code: "ABP-123", err: domain.E(domain.KindUpstream, "来源查询失败", nil), status: http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &discoverResolverStub{err: tc.err}
			router := NewRouter(Dependencies{Access: NewAccessGateService("", ""), Catalogue: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/discover/movies/resolve"+tc.query, nil))
			if response.Code != tc.status || stub.code != tc.code {
				t.Fatalf("status=%d code=%q body=%s", response.Code, stub.code, response.Body)
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if tc.status == http.StatusOK {
				if body["id"] != "confirmed-id" {
					t.Fatalf("identity: %v", body)
				}
			} else if _, exists := body["id"]; exists {
				t.Fatalf("failed resolution exposed an identity: %v", body)
			}
		})
	}
}
