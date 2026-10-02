package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gin-gonic/gin"
)

type controlledAccessGate struct {
	AccessGate
	verify func(string) error
}

func (g controlledAccessGate) Verify(password string) error { return g.verify(password) }

func TestAuthConcurrentAttemptsCannotBypassFailureLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hold := make(chan struct{})
		var verified atomic.Int32
		gate := controlledAccessGate{AccessGate: NewAccessGateService("correct", ""), verify: func(string) error {
			verified.Add(1)
			<-hold
			return ErrAccessPassword
		}}
		router := NewRouter(Dependencies{Access: gate, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		const attempts = 16
		finished := make(chan int, attempts)
		for range attempts {
			go func() {
				req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"wrong"}`))
				req.Header.Set("Content-Type", "application/json")
				req.RemoteAddr = "203.0.113.10:12345"
				response := httptest.NewRecorder()
				router.ServeHTTP(response, req)
				finished <- response.Code
			}()
		}
		synctest.Wait()
		if verified.Load() != 5 || len(finished) != attempts-5 {
			t.Errorf("verification calls=%d rejected=%d", verified.Load(), len(finished))
		}
		close(hold)
		statuses := make(map[int]int)
		for range attempts {
			statuses[<-finished]++
		}
		if statuses[http.StatusUnauthorized] != 5 || statuses[http.StatusTooManyRequests] != attempts-5 {
			t.Fatalf("unexpected responses: %v", statuses)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"correct"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.10:12345"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusTooManyRequests || verified.Load() != 5 {
			t.Fatalf("settled failures did not block: status=%d calls=%d", response.Code, verified.Load())
		}
	})
}

func TestAuthInvalidBodiesReleaseSlotsWithoutResettingFailures(t *testing.T) {
	limiter := newLoginRateLimiter(2, 5*time.Minute, time.Hour)
	router := gin.New()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router.Use(errorMiddleware(logger))
	router.POST("/login", accessLoginHandler(NewAccessGateService("correct", ""), limiter))
	request := func(body, ip string) int {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":12345"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response.Code
	}
	for _, body := range []string{`{}`, `{`, `{"password":""}`} {
		if status := request(body, "203.0.113.11"); status != http.StatusBadRequest || len(limiter.records) != 0 {
			t.Fatalf("invalid input retained a slot: status=%d records=%d", status, len(limiter.records))
		}
	}
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{"password":"wrong"}`, http.StatusUnauthorized},
		{`{}`, http.StatusBadRequest},
		{`{}`, http.StatusBadRequest},
		{`{"password":"wrong"}`, http.StatusUnauthorized},
		{`{"password":"correct"}`, http.StatusTooManyRequests},
	} {
		if status := request(test.body, "203.0.113.12"); status != test.status {
			t.Fatalf("body=%s status=%d want=%d", test.body, status, test.status)
		}
	}
	if record := limiter.records["203.0.113.12"]; record == nil || record.pending != 0 || record.failures != 2 {
		t.Fatalf("incorrect settled failure state: %+v", record)
	}
}
