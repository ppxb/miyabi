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

type retryTaskStub struct {
	TaskManager
	err error
	ids []int
}

type libraryControlStub struct {
	TaskManager
	paused []bool
}

func (s *libraryControlStub) SetLibraryPaused(_ context.Context, paused bool) error {
	s.paused = append(s.paused, paused)
	return nil
}

func TestLibraryPauseEndpoint(t *testing.T) {
	for _, tc := range []struct {
		body, password string
		status, calls  int
	}{
		{`{"paused":true}`, "", 202, 1}, {`{"paused":false}`, "", 202, 1}, {`{}`, "", 400, 0}, {`{"paused":null}`, "", 400, 0}, {`{"paused":true}`, "secret", 401, 0},
	} {
		stub := &libraryControlStub{}
		router := NewRouter(Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Tasks: stub, Access: NewAccessGateService(tc.password, "fixture-signing-key")})
		request := httptest.NewRequest(http.MethodPut, "/api/tasks/library-control", strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != tc.status || len(stub.paused) != tc.calls {
			t.Fatalf("pause endpoint: %d %s calls=%v", response.Code, response.Body, stub.paused)
		}
	}
}

func (s *retryTaskStub) Retry(_ context.Context, id int) (domain.TaskInfo, error) {
	s.ids = append(s.ids, id)
	return domain.TaskInfo{ID: id, Type: "scan", Status: "queued"}, s.err
}

func TestTaskRetryEndpoint(t *testing.T) {
	for _, test := range []struct {
		name, path, password string
		err                  error
		status, calls        int
	}{
		{name: "accepted", path: "/api/tasks/7/retry", status: 202, calls: 1},
		{name: "bad id", path: "/api/tasks/0/retry", status: 400},
		{name: "active task", path: "/api/tasks/7/retry", err: domain.E(domain.KindConflict, "任务正在处理中", nil), status: 409, calls: 1},
		{name: "authentication", path: "/api/tasks/7/retry", password: "secret", status: 401},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &retryTaskStub{err: test.err}
			router := NewRouter(Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Tasks: stub,
				Access: NewAccessGateService(test.password, "fixture-signing-key")})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, test.path, nil))
			if response.Code != test.status || len(stub.ids) != test.calls {
				t.Fatalf("retry = %d %s, calls=%v", response.Code, response.Body, stub.ids)
			}
		})
	}
}
