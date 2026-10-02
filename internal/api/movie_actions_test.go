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

type movieActionStub struct {
	LibraryManager
	id   int
	code string
	err  error
}

func (s *movieActionStub) RescrapeMovie(_ context.Context, id int, code string) (domain.TaskInfo, error) {
	s.id, s.code = id, code
	return domain.TaskInfo{ID: 7, Type: "scan", Status: "queued", MovieID: id, Code: code, Rebuild: true}, s.err
}

func TestMovieActionsValidateInputAndReturnQueuedWorkflow(t *testing.T) {
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{"42", `{}`, http.StatusAccepted},
		{"42", `{"code":"IPX-123"}`, http.StatusAccepted},
		{"0", `{}`, http.StatusBadRequest},
		{"bad", `{}`, http.StatusBadRequest},
		{"42", `{"code":3}`, http.StatusBadRequest},
		{"42", `{"code":"` + strings.Repeat("A", 121) + `"}`, http.StatusBadRequest},
	} {
		stub := &movieActionStub{}
		router := NewRouter(Dependencies{Access: NewAccessGateService("", ""), Library: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		request := httptest.NewRequest(http.MethodPost, "/api/library/movies/"+tc.path+"/scrape", strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Fatalf("%s %s: status=%d, %s", tc.path, tc.body, response.Code, response.Body)
		}
		if tc.status == http.StatusAccepted && (stub.id != 42 || !strings.Contains(response.Body.String(), `"movie_id":42`)) {
			t.Fatal("lost movie target")
		}
		if tc.status != http.StatusAccepted && stub.id != 0 {
			t.Fatal("invalid request reached service")
		}
	}
}
