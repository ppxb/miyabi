package pan

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestFileListDecodesNumericAndStringCountsAndSizes(t *testing.T) {
	for _, test := range []struct {
		name      string
		body      string
		wantTotal int
		wantSize  int64
	}{
		{
			name:      "numeric count and size",
			body:      `{"state":true,"code":0,"cid":"123","count":42,"data":[{"fid":"f1","pid":"123","fn":"movie.mp4","fc":"1","fs":1048576,"pc":"pick1","sha1":"abc"}],"path":[{"cid":"123","name":"media"}]}`,
			wantTotal: 42,
			wantSize:  1048576,
		},
		{
			name:      "string count and size",
			body:      `{"state":true,"code":0,"cid":"123","count":"99","data":[{"fid":"f2","pid":"123","fn":"movie2.mp4","fc":"1","fs":"2097152","pc":"pick2","sha1":"def"}],"path":[{"cid":"123","name":"media"}]}`,
			wantTotal: 99,
			wantSize:  2097152,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := New()
			defer client.Close()
			client.http.SetTransport(offlineRoundTrip(func(request *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": {"application/json"}},
					Body:       io.NopCloser(strings.NewReader(test.body)),
					Request:    request,
				}, nil
			}))

			page, err := client.List(t.Context(), "token", "123", 0, 10)
			if err != nil {
				t.Fatalf("List error = %v", err)
			}
			if page.Total != test.wantTotal {
				t.Errorf("Total = %d, want %d", page.Total, test.wantTotal)
			}
			if len(page.Files) != 1 || page.Files[0].Size != test.wantSize {
				t.Errorf("File size = %d, want %d", page.Files[0].Size, test.wantSize)
			}
		})
	}
}

func TestClientRetryPolicy(t *testing.T) {
	t.Run("POST does not retry on network error", func(t *testing.T) {
		client := New()
		defer client.Close()
		client.http.SetRetryWaitTime(1 * time.Millisecond)

		var calls int
		client.http.SetTransport(offlineRoundTrip(func(request *http.Request) (*http.Response, error) {
			calls++
			return nil, io.ErrUnexpectedEOF
		}))

		_, err := client.RefreshToken(t.Context(), "sample-refresh-token")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if calls != 1 {
			t.Fatalf("expected POST to not retry on error (calls = %d, want 1)", calls)
		}
	})

	t.Run("GET retries on network error", func(t *testing.T) {
		client := New()
		defer client.Close()
		client.http.SetRetryWaitTime(1 * time.Millisecond)

		var calls int
		client.http.SetTransport(offlineRoundTrip(func(request *http.Request) (*http.Response, error) {
			calls++
			if calls < 3 {
				return nil, io.ErrUnexpectedEOF
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"state":true,"code":0,"cid":"1","count":0,"data":[],"path":[{"cid":"1","name":"root"}]}`)),
				Request:    request,
			}, nil
		}))

		_, err := client.List(t.Context(), "token", "1", 0, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 3 {
			t.Fatalf("expected GET to retry (calls = %d, want 3)", calls)
		}
	})

	t.Run("POST retries on 429", func(t *testing.T) {
		client := New()
		defer client.Close()
		client.http.SetRetryWaitTime(1 * time.Millisecond)

		var calls int
		client.http.SetTransport(offlineRoundTrip(func(request *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Header:     http.Header{"Retry-After": {"0"}},
					Body:       io.NopCloser(strings.NewReader(`too many requests`)),
					Request:    request,
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"state":1,"code":0,"data":{"access_token":"a","refresh_token":"r","expires_in":3600}}`)),
				Request:    request,
			}, nil
		}))

		tokens, err := client.RefreshToken(t.Context(), "sample-refresh-token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 2 || tokens.AccessToken != "a" {
			t.Fatalf("expected POST to retry on 429 (calls = %d, want 2)", calls)
		}
	})
}

