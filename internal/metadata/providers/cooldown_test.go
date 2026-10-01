package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"golang.org/x/time/rate"
)

func TestSourceCooldownStopsOtherMoviesAndExpires(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(429)
			return
		}
		w.Write([]byte("available"))
	}))
	defer server.Close()
	c := newClient(nil, server.URL, "")
	c.http = server.Client()
	c.limiter = rate.NewLimiter(rate.Inf, 1)
	defer c.Close()
	for _, path := range []string{"/movie-a", "/movie-b", "/movie-c"} {
		_, err := c.get(t.Context(), server.URL+path)
		if delay, retry := domain.RetryDelay(err); !retry || delay < 119*time.Second {
			t.Fatalf("cooldown lost: %v %v %v", delay, retry, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("cooling source was hit for every movie")
	}
	c.cooldownUntil = time.Now().Add(-time.Second)
	if _, err := c.get(t.Context(), server.URL+"/recovered"); err != nil || calls.Load() != 2 {
		t.Fatalf("source did not recover: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.get(ctx, server.URL); err != context.Canceled {
		t.Fatalf("cancellation ignored: %v", err)
	}
}
