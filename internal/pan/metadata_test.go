package pan

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

func metadataTestClient(t *testing.T, media http.RoundTripper) *Client {
	t.Helper()
	client := New()
	t.Cleanup(client.Close)
	transport := client.http.GetClient().Transport.(*panTransport)
	transport.base = offlineRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "proapi.115.com" || req.URL.Path != "/open/ufile/downurl" || req.Method != http.MethodPost {
			t.Fatalf("unexpected API request: %s %s", req.Method, req.URL)
		}
		if req.UserAgent() != MediaUserAgent || req.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("downurl must use the media User-Agent and OAuth token")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"state":true,"code":0,"data":{"42":{"url":{"url":"https://cdn.example/sidecar?sign=fixture"}}}}`)),
		}, nil
	})
	client.media.Transport = media
	return client
}

func TestReadMetadataOnlyThrottlesDownloadURL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		var requestedAt []time.Duration
		client := metadataTestClient(t, offlineRoundTrip(func(req *http.Request) (*http.Response, error) {
			requestedAt = append(requestedAt, time.Since(start))
			if req.URL.String() != "https://cdn.example/sidecar?sign=fixture" || req.Method != http.MethodGet {
				t.Errorf("unexpected CDN request: %s %s", req.Method, req.URL)
			}
			if req.UserAgent() != MediaUserAgent || req.Header.Get("Authorization") != "" || req.Header.Get("Cookie") != "" {
				t.Error("CDN request must keep the media User-Agent without credentials")
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("nfo"))}, nil
		}))
		for range 2 {
			body, err := client.ReadMetadata(t.Context(), "fixture-token", "fixture-pick", 3)
			if err != nil || string(body) != "nfo" {
				t.Fatalf("body=%q err=%v", body, err)
			}
		}
		if len(requestedAt) != 2 || requestedAt[0] != 0 || requestedAt[1] != requestGap {
			t.Fatalf("expected only API requests to be throttled, CDN times=%v", requestedAt)
		}
		if client.media.Timeout != 0 {
			t.Fatal("sidecar timeout must not change video streaming timeout")
		}
	})
}

type metadataTestBody struct {
	io.Reader
	closed bool
}

func (b *metadataTestBody) Close() error { b.closed = true; return nil }

func TestReadMetadataLimitsAndPermanentErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		fail   bool
	}{
		{name: "below limit", status: 200, body: "nfo"},
		{name: "at limit", status: 200, body: "12345"},
		{name: "over limit", status: 200, body: "123456789", fail: true},
		{name: "not found", status: 404, body: "missing", fail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := strings.NewReader(test.body)
			responseBody := &metadataTestBody{Reader: reader}
			calls := 0
			client := metadataTestClient(t, offlineRoundTrip(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: test.status, Header: make(http.Header), Body: responseBody}, nil
			}))
			body, err := client.ReadMetadata(t.Context(), "fixture-token", "fixture-pick", 5)
			if (err != nil) != test.fail || !test.fail && string(body) != test.body {
				t.Fatalf("body=%q err=%v", body, err)
			}
			if test.fail && body != nil {
				t.Fatal("failed download returned partial metadata")
			}
			if test.status == 404 {
				var upstream *domain.HTTPError
				if !errors.As(err, &upstream) || upstream.StatusCode != 404 || upstream.Source != "115 metadata" {
					t.Fatalf("HTTP error lost its source or status: %v", err)
				}
			}
			if calls != 1 || !responseBody.closed {
				t.Fatalf("calls=%d body closed=%t", calls, responseBody.closed)
			}
			if test.name == "over limit" && reader.Len() != len(test.body)-6 {
				t.Fatal("read beyond the size limit plus one byte")
			}
		})
	}
}

func TestReadMetadataRetriesTransientDownloads(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     int
		err        error
		retryAfter string
		wait       time.Duration
	}{
		{name: "CDN backoff", status: 429, retryAfter: "2", wait: 2 * time.Second},
		{name: "server failure", status: 503, wait: time.Second},
		{name: "network failure", err: io.ErrUnexpectedEOF, wait: time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				start := time.Now()
				failedBody := &metadataTestBody{Reader: strings.NewReader("failure")}
				client := metadataTestClient(t, offlineRoundTrip(func(*http.Request) (*http.Response, error) {
					calls++
					if calls == 1 {
						if test.err != nil {
							return nil, test.err
						}
						return &http.Response{StatusCode: test.status, Header: http.Header{"Retry-After": {test.retryAfter}}, Body: failedBody}, nil
					}
					if test.err == nil && !failedBody.closed {
						t.Error("previous response body was not closed before retry")
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("nfo"))}, nil
				}))
				body, err := client.ReadMetadata(t.Context(), "fixture-token", "fixture-pick", 5)
				if err != nil || string(body) != "nfo" || calls != 2 || time.Since(start) != test.wait {
					t.Fatalf("body=%q calls=%d elapsed=%v err=%v", body, calls, time.Since(start), err)
				}
				transport := client.http.GetClient().Transport.(*panTransport)
				if !transport.retryAfter.IsZero() {
					t.Fatal("CDN Retry-After blocked unrelated API requests")
				}
			})
		})
	}
}

func TestReadMetadataBoundsHeadersAndBody(t *testing.T) {
	for _, stage := range []string{"headers", "body"} {
		t.Run(stage, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				var bodies []*blockedResponseBody
				start := time.Now()
				client := metadataTestClient(t, offlineRoundTrip(func(req *http.Request) (*http.Response, error) {
					calls++
					if deadline, ok := req.Context().Deadline(); !ok || deadline.Sub(time.Now()) != requestTimeout {
						t.Error("download did not receive a fresh timeout")
					}
					if stage == "headers" {
						<-req.Context().Done()
						return nil, req.Context().Err()
					}
					body := &blockedResponseBody{ctx: req.Context()}
					bodies = append(bodies, body)
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}, nil
				}))
				_, err := client.ReadMetadata(t.Context(), "fixture-token", "fixture-pick", 5)
				if !errors.Is(err, context.DeadlineExceeded) || calls != 4 || time.Since(start) != 4*requestTimeout+7*time.Second {
					t.Fatalf("calls=%d elapsed=%v err=%v", calls, time.Since(start), err)
				}
				for _, body := range bodies {
					if !body.closed {
						t.Error("timed out body was not closed")
					}
				}
			})
		})
	}
}

func TestReadMetadataCancellationDuringBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		client := metadataTestClient(t, offlineRoundTrip(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"60"}}, Body: http.NoBody}, nil
		}))
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go func() { time.Sleep(time.Second); cancel() }()
		start := time.Now()
		_, err := client.ReadMetadata(ctx, "fixture-token", "fixture-pick", 5)
		if !errors.Is(err, context.Canceled) || calls != 1 || time.Since(start) != time.Second {
			t.Fatalf("calls=%d elapsed=%v err=%v", calls, time.Since(start), err)
		}
	})
}
