package javbus

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"testing/synctest"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/ppxb/miyabi/internal/domain"
	"golang.org/x/time/rate"
)

type trackedResponseBody struct {
	io.Reader
	bytesRead int
	closed    bool
}

func (b *trackedResponseBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.bytesRead += n
	return n, err
}

func (b *trackedResponseBody) Close() error {
	b.closed = true
	return nil
}

func TestClientResponseSemantics(t *testing.T) {
	const challengeErr = "JavBus Cloudflare challenge encountered"
	const verifyErr = "JavBus driver verify required"
	for _, tc := range []struct {
		name, body, location     string
		status                   int
		page, detailErr, ajaxErr string
		unread                   bool
	}{
		{name: "success", status: 200, body: "<html>ok</html>", page: "<html>ok</html>"},
		{name: "not_found_status", status: 404, body: "not found", ajaxErr: "JavBus returned unexpected status 404"},
		{name: "not_found_content", status: 200, body: "404 Page Not Found"},
		{name: "not_found_before_challenge", status: 404, body: "challenge-platform", ajaxErr: challengeErr},
		{name: "not_found_content_before_forbidden", status: 403, body: "404 Page Not Found", ajaxErr: challengeErr},
		{name: "verify_redirect", status: 301, location: baseURL + "/doc/driver-verify", detailErr: verifyErr, ajaxErr: verifyErr, unread: true},
		{name: "other_redirect", status: 302, location: baseURL + "/other", detailErr: "JavBus unexpected redirect to " + baseURL + "/other", ajaxErr: verifyErr, unread: true},
		{name: "temporary_redirect", status: 307, body: "redirecting", detailErr: "JavBus returned unexpected status 307", ajaxErr: "JavBus returned unexpected status 307"},
		{name: "forbidden", status: 403, detailErr: challengeErr, ajaxErr: challengeErr},
		{name: "unavailable", status: 503, detailErr: challengeErr, ajaxErr: challengeErr},
		{name: "challenge_before_verify", status: 200, body: "challenge-platform driver-verify", detailErr: challengeErr, ajaxErr: challengeErr},
		{name: "verify_content", status: 200, body: "driver-verify", detailErr: verifyErr, ajaxErr: verifyErr},
		{name: "server_error", status: 500, detailErr: "JavBus returned unexpected status 500", ajaxErr: "JavBus returned unexpected status 500"},
	} {
		for _, stage := range []string{"detail", "ajax"} {
			t.Run(tc.name+"/"+stage, func(t *testing.T) {
				body := &trackedResponseBody{Reader: strings.NewReader(tc.body)}
				if tc.unread {
					body.Reader = iotest.ErrReader(errors.New("redirect body must not be read"))
				}
				mock := newMockHTTPClient()
				handler := func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: tc.status, Body: body, Header: http.Header{"Location": {tc.location}}}, nil
				}
				mock.handlers["/SSIS-001"] = handler
				mock.handlers["/ajax/uncledatoolsbyajax.php"] = handler
				client := NewForTest(true, mock)
				defer client.Close()
				// Use production availability handling without background probes.
				client.isTest = false
				var err error
				wantErr := tc.detailErr
				if stage == "detail" {
					var page string
					page, err = client.MoviePage(t.Context(), "SSIS-001")
					if page != tc.page {
						t.Errorf("page = %q, want %q", page, tc.page)
					}
				} else {
					wantErr = tc.ajaxErr
					_, err = client.fetchMagnets(t.Context(), "SSIS-001", "123", "0", "cover.jpg")
				}
				if wantErr == "" {
					if err != nil {
						t.Errorf("unexpected error: %v", err)
					}
				} else if err == nil || err.Error() != wantErr || !domain.IsKind(err, domain.KindUpstream) {
					t.Errorf("error = %v, want upstream error %q", err, wantErr)
				}
				if wantAvailable := wantErr != challengeErr; client.Available() != wantAvailable {
					t.Errorf("available = %t, want %t", client.Available(), wantAvailable)
				}
				if !body.closed {
					t.Error("response body was not closed")
				}
			})
		}
	}
}

func TestClientResponseReadLimit(t *testing.T) {
	const limit = 2 << 20
	for _, stage := range []string{"detail", "ajax"} {
		t.Run(stage, func(t *testing.T) {
			// Content past the limit must neither be read nor affect validation.
			body := &trackedResponseBody{Reader: strings.NewReader(strings.Repeat(" ", limit) + "challenge-platform")}
			mock := newMockHTTPClient()
			handler := func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: body}, nil
			}
			mock.handlers["/SSIS-001"] = handler
			mock.handlers["/ajax/uncledatoolsbyajax.php"] = handler
			client := NewForTest(true, mock)
			defer client.Close()
			var err error
			if stage == "detail" {
				var page string
				page, err = client.MoviePage(t.Context(), "SSIS-001")
				if len(page) != limit {
					t.Errorf("page length = %d, want %d", len(page), limit)
				}
			} else {
				_, err = client.fetchMagnets(t.Context(), "SSIS-001", "123", "0", "cover.jpg")
			}
			if err != nil || body.bytesRead != limit || !body.closed {
				t.Fatalf("error = %v, bytes read = %d, closed = %t", err, body.bytesRead, body.closed)
			}
		})
	}
}

func TestClientRequestAndReadErrors(t *testing.T) {
	for _, stage := range []string{"detail", "ajax magnets"} {
		for _, failure := range []string{"request", "read"} {
			t.Run(stage+"/"+failure, func(t *testing.T) {
				cause := errors.New("upstream interrupted")
				body := &trackedResponseBody{Reader: iotest.ErrReader(cause)}
				mock := newMockHTTPClient()
				handler := func(*http.Request) (*http.Response, error) {
					if failure == "request" {
						return nil, cause
					}
					return &http.Response{StatusCode: 200, Body: body}, nil
				}
				mock.handlers["/SSIS-001"] = handler
				mock.handlers["/ajax/uncledatoolsbyajax.php"] = handler
				client := NewForTest(true, mock)
				defer client.Close()
				client.isTest = false
				var err error
				if stage == "detail" {
					_, err = client.MoviePage(t.Context(), "SSIS-001")
				} else {
					_, err = client.fetchMagnets(t.Context(), "SSIS-001", "123", "0", "cover.jpg")
				}
				wantMessage := "JavBus " + stage + " request failed"
				if failure == "read" {
					wantMessage = "read JavBus " + stage + " response"
				}
				if !errors.Is(err, cause) || !domain.IsKind(err, domain.KindUpstream) || domain.PublicMessage(err) != wantMessage {
					t.Errorf("error = %v, want upstream error %q wrapping cause", err, wantMessage)
				}
				if client.Available() != (failure == "read") || body.closed != (failure == "read") {
					t.Errorf("available = %t, closed = %t", client.Available(), body.closed)
				}
			})
		}
	}
}

func TestClientSharedRequestLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mock := newMockHTTPClient()
		client := NewForTest(true, mock)
		defer client.Close()
		client.limiter = rate.NewLimiter(1, 1)
		start := time.Now()
		_, _ = client.MoviePage(t.Context(), "SSIS-001")
		_, _ = client.fetchMagnets(t.Context(), "SSIS-001", "123", "0", "cover.jpg")
		if elapsed := time.Since(start); elapsed != time.Second {
			t.Fatalf("elapsed = %v, want shared limit of 1s", elapsed)
		}

		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
		}()
		_, err := client.MoviePage(ctx, "SSIS-001")
		if !errors.Is(err, context.Canceled) || mock.getCallCount("/SSIS-001") != 1 || !client.Available() {
			t.Fatalf("error = %v, detail calls = %d, available = %t", err, mock.getCallCount("/SSIS-001"), client.Available())
		}
	})
}
