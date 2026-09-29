package javdb

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	"golang.org/x/time/rate"
)

type retryTransport func() error

func (f retryTransport) getJSON(context.Context, string, url.Values, any) error { return f() }
func (retryTransport) closeIdleConnections()                                    {}

func TestRetryAfterFormats(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		value string
		want  time.Duration
	}{
		{" 60 ", time.Minute}, {now.Add(time.Minute).Format(http.TimeFormat), time.Minute},
		{"", 0}, {"0", 0}, {"-1", 0}, {"invalid", 0}, {"1.5", 0},
		{now.Add(-time.Minute).Format(http.TimeFormat), 0}, {"18446744073709551615", time.Duration(math.MaxInt64)},
	} {
		if got := parseRetryAfter(test.value, now); got != test.want {
			t.Errorf("Retry-After %q = %v, want %v", test.value, got, test.want)
		}
	}
	tr := &transport{host: "https://fixture.example", client: &responseClient{response: &fhttp.Response{
		StatusCode: 429, Header: fhttp.Header{"Retry-After": []string{"60"}}, Body: io.NopCloser(strings.NewReader("limited")),
	}}}
	var response *HTTPError
	if err := tr.getJSON(t.Context(), "/test", nil, nil); !errors.As(err, &response) || response.RetryAfter != time.Minute {
		t.Fatalf("transport discarded Retry-After: %v", err)
	}
}

func TestClientRetries429OnSameRoute(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		var calls []time.Duration
		client := &Client{limiter: rate.NewLimiter(rate.Inf, 1)}
		client.current.Store(&routeState{transport: retryTransport(func() error {
			calls = append(calls, time.Since(start))
			if len(calls) == 1 {
				return &HTTPError{StatusCode: 429, RetryAfter: 5 * time.Second}
			}
			if len(calls) == 2 {
				return &HTTPError{StatusCode: 429, RetryAfter: 2 * time.Second}
			}
			return nil
		})})
		client.selector = func(context.Context, routeSelection) (*routeState, error) {
			t.Fatal("429 caused route failover")
			return nil, nil
		}
		if err := client.getJSON(t.Context(), "/test", nil, nil); err != nil {
			t.Fatal(err)
		}
		if len(calls) != 3 || calls[1] != 5*time.Second || calls[2] != 7*time.Second {
			t.Fatalf("retry timing = %v", calls)
		}
	})
}

func TestClientLimits429RetriesAndHonorsLongCooldown(t *testing.T) {
	for _, delay := range []time.Duration{0, 3 * time.Minute} {
		t.Run(delay.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				start := time.Now()
				client := &Client{limiter: rate.NewLimiter(rate.Inf, 1)}
				client.current.Store(&routeState{transport: retryTransport(func() error { calls++; return &HTTPError{StatusCode: 429, RetryAfter: delay} })})
				var response *HTTPError
				if err := client.getJSON(t.Context(), "/test", nil, nil); !errors.As(err, &response) || response.StatusCode != 429 {
					t.Fatalf("limit error = %v", err)
				}
				if delay == 0 {
					if calls != 4 || time.Since(start) < 7*time.Second || time.Since(start) >= 11*time.Second {
						t.Fatalf("unbounded or immediate retries: %d, %v", calls, time.Since(start))
					}
				} else {
					if calls != 1 || time.Since(start) != 0 {
						t.Fatalf("long Retry-After was truncated: %d, %v", calls, time.Since(start))
					}
					if err := client.getJSON(t.Context(), "/another", nil, nil); !errors.As(err, &response) || calls != 1 {
						t.Fatalf("another caller ignored cooldown: %d, %v", calls, err)
					}
				}
			})
		})
	}
}

func TestClientCooldownIsSharedExtendedAndCancelable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := &Client{limiter: rate.NewLimiter(rate.Every(time.Second), 1)}
		calls := make(chan time.Time, 4)
		client.current.Store(&routeState{transport: retryTransport(func() error { calls <- time.Now(); return nil })})
		start := time.Now()
		client.deferRequests(10 * time.Second)
		ctx, cancel := context.WithCancel(t.Context())
		finished := make(chan error, 3)
		go func() { finished <- client.getJSON(ctx, "/canceled", nil, nil) }()
		synctest.Wait()
		cancel()
		if err := <-finished; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel cooldown: %v", err)
		}
		for range 2 {
			go func() { finished <- client.getJSON(t.Context(), "/test", nil, nil) }()
		}
		synctest.Wait()
		time.Sleep(5 * time.Second)
		client.deferRequests(10 * time.Second)
		client.deferRequests(time.Second)
		time.Sleep(5 * time.Second)
		synctest.Wait()
		if len(calls) != 0 {
			t.Fatal("a caller escaped the extended cooldown")
		}
		for range 2 {
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
		}
		first, second := <-calls, <-calls
		if first.Sub(start) != 15*time.Second || second.Sub(first) < time.Second {
			t.Fatalf("cooldown ended in a burst: %v %v", first.Sub(start), second.Sub(start))
		}
	})
}

func TestClientRechecksCooldownAfterWaitingForRateToken(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := &Client{limiter: rate.NewLimiter(rate.Every(time.Second), 1)}
		calls := make(chan time.Time, 4)
		count := 0
		start := time.Now()
		client.current.Store(&routeState{transport: retryTransport(func() error {
			count++
			calls <- time.Now()
			if count == 1 {
				time.Sleep(100 * time.Millisecond)
				return &HTTPError{StatusCode: 429, RetryAfter: 10 * time.Second}
			}
			return nil
		})})
		finished := make(chan error, 2)
		for range 2 {
			go func() { finished <- client.getJSON(t.Context(), "/test", nil, nil) }()
		}
		for range 2 {
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
		}
		<-calls
		if second := <-calls; second.Sub(start) < 10100*time.Millisecond {
			t.Fatalf("rate waiter skipped cooldown: %v", second.Sub(start))
		}
	})
}
