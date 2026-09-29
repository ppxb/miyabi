package pan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/go-resty/resty/v2"
	"golang.org/x/time/rate"
)

func TestPanTransport_RateLimitsRetries(t *testing.T) {
	var requestTimes []time.Time
	var mu sync.Mutex

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestTimes = append(requestTimes, time.Now())
		count := len(requestTimes)
		mu.Unlock()

		if count < 3 {
			// Fail first 2 attempts with 502 to trigger Resty retry
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"state":1,"code":0,"data":{"ok":true}}`))
	}))
	defer ts.Close()

	gap := 100 * time.Millisecond
	limiter := rate.NewLimiter(rate.Every(gap), 1)

	restyClient := resty.New()
	transport := newPanTransport(http.DefaultTransport, limiter, 2)
	restyClient.SetTransport(transport)
	restyClient.SetRetryCount(3)
	restyClient.SetRetryWaitTime(10 * time.Millisecond) // Short retry wait to test limiter constraint
	restyClient.AddRetryCondition(func(r *resty.Response, err error) bool {
		return r != nil && r.StatusCode() == http.StatusBadGateway
	})

	client := &Client{
		http:    restyClient,
		limiter: limiter,
	}

	req := client.http.R().SetContext(context.Background())
	_, err := client.request(req, http.MethodGet, ts.URL)
	if err != nil {
		t.Fatalf("expected request success after retries, got: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requestTimes) != 3 {
		t.Fatalf("expected 3 attempts, got %d", len(requestTimes))
	}

	for i := 1; i < len(requestTimes); i++ {
		diff := requestTimes[i].Sub(requestTimes[i-1])
		// Even though resty retry wait time was 10ms, panTransport limiter must enforce >= 90ms
		if diff < 90*time.Millisecond {
			t.Errorf("attempt %d -> %d interval %v was shorter than limiter gap %v", i-1, i, diff, gap)
		}
	}
}

func TestPanTransport_LimitsInFlightConcurrency(t *testing.T) {
	var currentInFlight int64
	var maxObservedInFlight int64

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt64(&currentInFlight, 1)
		for {
			max := atomic.LoadInt64(&maxObservedInFlight)
			if cur <= max || atomic.CompareAndSwapInt64(&maxObservedInFlight, max, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt64(&currentInFlight, -1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"state":1,"code":0,"data":{}}`))
	}))
	defer ts.Close()

	// High rate limit so rate limiter won't bottleneck in-flight test
	limiter := rate.NewLimiter(rate.Inf, 1)
	restyClient := resty.New()
	transport := newPanTransport(http.DefaultTransport, limiter, 2)
	restyClient.SetTransport(transport)

	client := &Client{
		http:    restyClient,
		limiter: limiter,
	}

	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := client.http.R().SetContext(context.Background())
			_, _ = client.request(req, http.MethodGet, ts.URL)
		}()
	}
	wg.Wait()

	max := atomic.LoadInt64(&maxObservedInFlight)
	if max > 2 {
		t.Errorf("max in-flight was %d, expected <= 2", max)
	}
}

func TestPanTransport_RespectsRetryAfter(t *testing.T) {
	var attempts int64
	start := time.Now()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		att := atomic.AddInt64(&attempts, 1)
		if att == 1 {
			w.Header().Set("Retry-After", "1") // 1 second
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"state":1,"code":0,"data":{}}`))
	}))
	defer ts.Close()

	limiter := rate.NewLimiter(rate.Inf, 1)
	restyClient := resty.New()
	transport := newPanTransport(http.DefaultTransport, limiter, 2)
	restyClient.SetTransport(transport)
	restyClient.SetRetryCount(2)
	restyClient.SetRetryMaxWaitTime(10 * time.Second)
	restyClient.SetRetryWaitTime(10 * time.Millisecond)
	restyClient.AddRetryCondition(func(r *resty.Response, err error) bool {
		return r != nil && r.StatusCode() == http.StatusTooManyRequests
	})

	client := &Client{
		http:    restyClient,
		limiter: limiter,
	}

	req := client.http.R().SetContext(context.Background())
	_, err := client.request(req, http.MethodGet, ts.URL)
	if err != nil {
		t.Fatalf("expected request success, got: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed < 950*time.Millisecond {
		t.Errorf("retry happened too early (%v), did not respect Retry-After: 1s", elapsed)
	}
}

func TestPanTransportRechecksExtendedDeadline(t *testing.T) {
	for _, stage := range []string{"backoff", "rate limit"} {
		t.Run(stage, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				limiter := rate.NewLimiter(rate.Inf, 1)
				if stage == "rate limit" {
					limiter = rate.NewLimiter(rate.Every(2*time.Second), 1)
					limiter.Allow()
				}
				called := make(chan time.Time, 1)
				transport := newPanTransport(offlineRoundTrip(func(*http.Request) (*http.Response, error) {
					called <- time.Now()
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: http.NoBody}, nil
				}), limiter, 2)
				start := time.Now()
				if stage == "backoff" {
					transport.retryAfter = start.Add(2 * time.Second)
				}
				done := make(chan error, 1)
				req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com", nil)
				go func() {
					resp, err := transport.RoundTrip(req)
					if resp != nil {
						resp.Body.Close()
					}
					done <- err
				}()
				synctest.Wait()
				time.Sleep(time.Second)
				transport.mu.Lock()
				transport.retryAfter = start.Add(5 * time.Second)
				transport.mu.Unlock()
				time.Sleep(time.Second)
				synctest.Wait()
				select {
				case <-called:
					t.Fatal("request escaped through the old deadline")
				default:
				}
				time.Sleep(3 * time.Second)
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				if at := <-called; at.Before(start.Add(5 * time.Second)) {
					t.Fatalf("request started at %v", at)
				}
			})
		})
	}
}

func TestPanTransportCancelBackoffReleasesSlot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		transport := newPanTransport(offlineRoundTrip(func(*http.Request) (*http.Response, error) {
			t.Error("canceled backoff reached network")
			return nil, errors.New("unexpected request")
		}), rate.NewLimiter(rate.Inf, 1), 1)
		transport.retryAfter = time.Now().Add(time.Minute)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", nil)
		done := make(chan error, 1)
		go func() { _, err := transport.RoundTrip(req); done <- err }()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation=%v", err)
		}
		if len(transport.inFlight) != 0 {
			t.Fatal("cancellation leaked concurrency slot")
		}
	})
}

func TestPanClientRetryAfterUsesSharedTransport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := New()
		defer client.Close()
		transport := client.http.GetClient().Transport.(*panTransport)
		attempts := 0
		start := time.Now()
		transport.base = offlineRoundTrip(func(*http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"5"}}, Body: http.NoBody}, nil
			}
			if time.Since(start) != 5*time.Second {
				t.Errorf("retry delay=%v", time.Since(start))
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: http.NoBody}, nil
		})
		if _, err := client.request(client.http.R().SetContext(t.Context()), http.MethodGet, "http://example.com"); err != nil {
			t.Fatal(err)
		}
		if attempts != 2 {
			t.Fatalf("attempts=%d", attempts)
		}
	})
}

func TestPanClientLongRetryAfterDoesNotConsumeIOTimeout(t *testing.T) {
	for _, seconds := range []int{60, 120} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			t.Run(fmt.Sprintf("%s/%d", method, seconds), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					client := New()
					defer client.Close()
					transport := client.http.GetClient().Transport.(*panTransport)
					attempts := 0
					start := time.Now()
					transport.base = offlineRoundTrip(func(req *http.Request) (*http.Response, error) {
						attempts++
						if deadline, ok := req.Context().Deadline(); !ok || deadline.Sub(time.Now()) != requestTimeout {
							t.Error("network attempt did not receive a fresh I/O deadline")
						}
						if attempts == 1 {
							return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {fmt.Sprint(seconds)}}, Body: http.NoBody}, nil
						}
						return &http.Response{StatusCode: 200, Header: make(http.Header), Body: http.NoBody}, nil
					})
					_, err := client.request(client.http.R().SetContext(t.Context()), method, "http://example.com")
					if err != nil || attempts != 2 || time.Since(start) != time.Duration(seconds)*time.Second {
						t.Fatalf("attempts=%d elapsed=%s err=%v", attempts, time.Since(start), err)
					}
				})
			})
		}
	}
}

type blockedResponseBody struct {
	ctx    context.Context
	closed bool
}

func (b *blockedResponseBody) Read([]byte) (int, error) { <-b.ctx.Done(); return 0, b.ctx.Err() }
func (b *blockedResponseBody) Close() error             { b.closed = true; return nil }

func TestPanClientStillBoundsHeadersAndBody(t *testing.T) {
	for _, stage := range []string{"headers", "body"} {
		t.Run(stage, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				client := New()
				defer client.Close()
				transport := client.http.GetClient().Transport.(*panTransport)
				var body *blockedResponseBody
				attempts := 0
				start := time.Now()
				transport.base = offlineRoundTrip(func(req *http.Request) (*http.Response, error) {
					attempts++
					if stage == "headers" {
						<-req.Context().Done()
						return nil, req.Context().Err()
					}
					body = &blockedResponseBody{ctx: req.Context()}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}, nil
				})
				_, err := client.request(client.http.R().SetContext(t.Context()), http.MethodPost, "http://example.com")
				if !errors.Is(err, context.DeadlineExceeded) || attempts != 1 || time.Since(start) != requestTimeout {
					t.Fatalf("attempts=%d elapsed=%s err=%v", attempts, time.Since(start), err)
				}
				if body != nil && !body.closed {
					t.Fatal("timed out body not closed")
				}
			})
		})
	}
}

func TestPanResponseDeadlineEndsAtEOFOrClose(t *testing.T) {
	for _, read := range []bool{false, true} {
		t.Run(fmt.Sprint(read), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var networkCtx context.Context
				transport := newPanTransport(offlineRoundTrip(func(req *http.Request) (*http.Response, error) {
					networkCtx = req.Context()
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: http.NoBody}, nil
				}), rate.NewLimiter(rate.Inf, 1), 1)
				req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com", nil)
				resp, err := transport.RoundTrip(req)
				if err != nil {
					t.Fatal(err)
				}
				if networkCtx.Err() != nil {
					t.Fatal("deadline canceled before body consumption")
				}
				if read {
					if _, err := io.ReadAll(resp.Body); err != nil {
						t.Fatal(err)
					}
				} else {
					resp.Body.Close()
				}
				if !errors.Is(networkCtx.Err(), context.Canceled) {
					t.Fatal("finished response retained its timer")
				}
				resp.Body.Close()
			})
		})
	}
}
