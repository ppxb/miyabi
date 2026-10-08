package javbus

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/magnet"
	"github.com/ppxb/miyabi/internal/netx"
)

type mockHTTPClient struct {
	mu       sync.Mutex
	handlers map[string]func(req *http.Request) (*http.Response, error)
	calls    map[string]int
}

func newMockHTTPClient() *mockHTTPClient {
	return &mockHTTPClient{
		handlers: make(map[string]func(req *http.Request) (*http.Response, error)),
		calls:    make(map[string]int),
	}
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	reqPath := req.URL.Path
	m.calls[reqPath]++

	if handler, ok := m.handlers[reqPath]; ok {
		return handler(req)
	}

	return &http.Response{
		StatusCode: 404,
		Body:       io.NopCloser(strings.NewReader("not found")),
		Header:     make(http.Header),
	}, nil
}

func (m *mockHTTPClient) CloseIdleConnections() {}

func (m *mockHTTPClient) getCallCount(path string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[path]
}

func TestClient_FindUsesFreshDetailParameters(t *testing.T) {
	mock := newMockHTTPClient()
	detailBody := fixtureFile(t, "detail_ssis-001.html")
	magnetsBody := fixtureFile(t, "magnets_ssis-001.html")
	expectedGID := "45622804531"

	mock.handlers["/SSIS-001"] = func(req *http.Request) (*http.Response, error) {
		for key, want := range map[string]string{
			"User-Agent":       userAgent,
			"Cookie":           "dv=1; existmag=all",
			"Accept":           "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			"Referer":          "",
			"X-Requested-With": "",
		} {
			if got := req.Header.Get(key); got != want {
				t.Errorf("detail header %s = %q, want %q", key, got, want)
			}
		}
		if req.URL.Query().Get("existmag") != "all" {
			t.Errorf("expected existmag=all query parameter")
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(detailBody)),
			Header:     make(http.Header),
		}, nil
	}

	mock.handlers["/ajax/uncledatoolsbyajax.php"] = func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("gid") != expectedGID {
			t.Errorf("unexpected gid query: %s", req.URL.Query().Get("gid"))
		}
		if req.URL.Query().Get("uc") != "0" {
			t.Errorf("unexpected uc query: %s", req.URL.Query().Get("uc"))
		}
		for key, want := range map[string]string{
			"User-Agent":       userAgent,
			"Cookie":           "dv=1; existmag=all",
			"Accept":           "",
			"Referer":          baseURL + "/SSIS-001",
			"X-Requested-With": "XMLHttpRequest",
		} {
			if got := req.Header.Get(key); got != want {
				t.Errorf("ajax header %s = %q, want %q", key, got, want)
			}
		}
		if floor, err := strconv.Atoi(req.URL.Query().Get("floor")); err != nil || floor < 1 || floor > 1000 {
			t.Errorf("unexpected floor query: %s", req.URL.Query().Get("floor"))
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(magnetsBody)),
			Header:     make(http.Header),
		}, nil
	}

	client, err := New(Options{testClient: mock})
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}
	defer client.Close()

	if client.Name() != "javbus" {
		t.Fatalf("expected name to be javbus, got %s", client.Name())
	}

	// Each upstream lookup obtains the current detail parameters.
	magnets, err := client.Find(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if len(magnets) != 43 {
		t.Fatalf("expected 43 magnets, got %d", len(magnets))
	}
	if mock.getCallCount("/SSIS-001") != 1 {
		t.Fatalf("expected 1 detail call, got %d", mock.getCallCount("/SSIS-001"))
	}
	if mock.getCallCount("/ajax/uncledatoolsbyajax.php") != 1 {
		t.Fatalf("expected 1 ajax call, got %d", mock.getCallCount("/ajax/uncledatoolsbyajax.php"))
	}

	// The next lookup must use updated detail parameters rather than stale ones.
	expectedGID = "45622804532"
	detailBody = bytes.ReplaceAll(detailBody, []byte("45622804531"), []byte(expectedGID))
	magnets2, err := client.Find(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if err != nil {
		t.Fatalf("second Find failed: %v", err)
	}
	if len(magnets2) != 43 {
		t.Fatalf("expected 43 magnets, got %d", len(magnets2))
	}
	if mock.getCallCount("/SSIS-001") != 2 {
		t.Fatalf("expected 2 detail calls, got %d", mock.getCallCount("/SSIS-001"))
	}
	if mock.getCallCount("/ajax/uncledatoolsbyajax.php") != 2 {
		t.Fatalf("expected 2 ajax calls, got %d", mock.getCallCount("/ajax/uncledatoolsbyajax.php"))
	}
}

func TestClientContextErrorsDoNotDisableSource(t *testing.T) {
	for _, stage := range []string{"detail", "magnets"} {
		for _, canceled := range []bool{true, false} {
			name := "timeout"
			if canceled {
				name = "canceled"
			}
			t.Run(stage+"/"+name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(t.Context(), time.Second)
					defer cancel()
					mock := newMockHTTPClient()
					mock.handlers["/SSIS-001"] = func(*http.Request) (*http.Response, error) {
						return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(fixtureFile(t, "detail_ssis-001.html")))}, nil
					}
					requestPath := "/SSIS-001"
					if stage == "magnets" {
						requestPath = "/ajax/uncledatoolsbyajax.php"
					}
					mock.handlers[requestPath] = func(req *http.Request) (*http.Response, error) {
						if canceled {
							cancel()
						}
						<-req.Context().Done()
						return nil, errors.New("transport interrupted")
					}
					client, err := New(Options{testClient: mock})
					if err != nil {
						t.Fatal(err)
					}
					defer client.Close()
					// Exercise production availability handling without background probes.
					client.isTest = false
					_, err = client.Find(ctx, domain.MovieRef{Code: "SSIS-001"})
					want := context.DeadlineExceeded
					if canceled {
						want = context.Canceled
					}
					if err != want || !client.Available() || mock.getCallCount(requestPath) != 1 {
						t.Fatalf("err=%v available=%t calls=%d", err, client.Available(), mock.getCallCount(requestPath))
					}
				})
			})
		}
	}
}

func TestClient_CategoryAndPrefixSkipping(t *testing.T) {
	mock := newMockHTTPClient()
	client, err := New(Options{
		testClient: mock,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Close()

	cases := []domain.MovieRef{
		{Code: "SSIS-001", Zone: domain.ZoneWestern},
		{Code: "SSIS-001", Zone: domain.ZoneAnime},
		{Code: "SSIS-001", Zone: domain.ZoneFC2},
		{Code: "FC2-PPV-1234567"},
		{Code: "fc2-123456"},
		{Code: "  "},
		{Code: ""},
	}

	for _, ref := range cases {
		magnets, err := client.Find(t.Context(), ref)
		if err != nil {
			t.Errorf("expected no error for ref %+v, got %v", ref, err)
		}
		if len(magnets) != 0 {
			t.Errorf("expected no magnets for ref %+v, got %d", ref, len(magnets))
		}
	}

	if len(mock.calls) != 0 {
		t.Errorf("expected zero HTTP requests made, got %v", mock.calls)
	}
}

func TestClient_NotFound(t *testing.T) {
	mock := newMockHTTPClient()
	notFoundBody := fixtureFile(t, "notfound_zzzz-99999.html")

	mock.handlers["/ZZZZ-99999"] = func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(bytes.NewReader(notFoundBody)),
			Header:     make(http.Header),
		}, nil
	}

	client, err := New(Options{
		testClient: mock,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Close()

	magnets, err := client.Find(t.Context(), domain.MovieRef{Code: "ZZZZ-99999"})
	if err != nil {
		t.Fatalf("expected nil error for not found, got %v", err)
	}
	if len(magnets) != 0 {
		t.Fatalf("expected empty magnets slice, got %d", len(magnets))
	}
}

func TestClient_DriverVerifyRedirect(t *testing.T) {
	mock := newMockHTTPClient()
	mock.handlers["/BLOCKED-001"] = func(req *http.Request) (*http.Response, error) {
		header := make(http.Header)
		header.Set("Location", "https://www.javbus.com/doc/driver-verify")
		return &http.Response{
			StatusCode: 302,
			Body:       io.NopCloser(strings.NewReader("redirecting")),
			Header:     header,
		}, nil
	}

	client, err := New(Options{
		testClient: mock,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Close()

	_, err = client.Find(t.Context(), domain.MovieRef{Code: "BLOCKED-001"})
	if err == nil {
		t.Fatalf("expected error for driver-verify redirect")
	}
	if !domain.IsKind(err, domain.KindUpstream) {
		t.Fatalf("expected KindUpstream error, got %v", err)
	}
	if !strings.Contains(err.Error(), "driver verify") {
		t.Fatalf("expected error message to mention driver verify, got %v", err)
	}
}

func TestClient_CloudflareChallenge(t *testing.T) {
	mock := newMockHTTPClient()
	mock.handlers["/CF-001"] = func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 403,
			Body:       io.NopCloser(strings.NewReader("<title>Just a moment...</title>")),
			Header:     make(http.Header),
		}, nil
	}

	client, err := New(Options{
		testClient: mock,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Close()

	_, err = client.Find(t.Context(), domain.MovieRef{Code: "CF-001"})
	if err == nil {
		t.Fatalf("expected error for cloudflare challenge")
	}
	if !domain.IsKind(err, domain.KindUpstream) {
		t.Fatalf("expected KindUpstream error, got %v", err)
	}
	if !strings.Contains(err.Error(), "Cloudflare") {
		t.Fatalf("expected error message to mention Cloudflare, got %v", err)
	}
}

func TestClient_ProxyManagerIntegration(t *testing.T) {
	manager, err := netx.NewProxyManager(netx.ProxyConfig{})
	if err != nil {
		t.Fatalf("unexpected error creating proxy manager: %v", err)
	}

	client, err := New(Options{
		Proxy: manager,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Close()

	// Updating proxy should notify watchProxy without panic or hang.
	manager.Update(netx.ProxyConfig{
		Enabled: true,
		URL:     "http://127.0.0.1:18888",
	})

	time.Sleep(50 * time.Millisecond)

	parsed, _ := url.Parse("http://127.0.0.1:18888")
	if manager.Resolve().String() != parsed.String() {
		t.Fatalf("expected proxy to be resolved as %v, got %v", parsed, manager.Resolve())
	}
}

func TestClient_AvailabilityAndSkipping(t *testing.T) {
	mock := newMockHTTPClient()
	client, err := New(Options{testClient: mock})
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}
	defer client.Close()

	if !client.Available() {
		t.Fatal("expected testClient to start as available")
	}

	// Flip available to false; Find must return magnet.ErrSkipped with 0 calls made.
	client.setAvailable(false)
	if client.Available() {
		t.Fatal("expected Available() to be false")
	}

	magnets, err := client.Find(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if !errors.Is(err, magnet.ErrSkipped) {
		t.Fatalf("expected ErrSkipped when unavailable, got %v", err)
	}
	if len(magnets) != 0 {
		t.Fatalf("expected nil magnets, got %d", len(magnets))
	}
	if len(mock.calls) != 0 {
		t.Fatalf("expected zero HTTP requests when skipped, got %v", mock.calls)
	}

	// Restore availability; requests proceed.
	client.setAvailable(true)
	if !client.Available() {
		t.Fatal("expected Available() to be true")
	}
}
