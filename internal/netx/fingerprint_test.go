package netx

import (
	"errors"
	"io"
	"net"
	stdhttp "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	http "github.com/bogdanfinn/fhttp"
)

func TestProxiedFingerprintClientSwitchesProxyAndCloses(t *testing.T) {
	server := func(label string) *httptest.Server {
		return httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			_, _ = io.WriteString(w, label)
		}))
	}
	direct, first, second := server("direct"), server("first"), server("second")
	defer direct.Close()
	defer first.Close()
	defer second.Close()
	manager, err := NewProxyManager(ProxyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewProxiedFingerprintClient(manager, FingerprintOptions{Timeout: time.Second, CookieJar: true})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	read := func(want string) {
		t.Helper()
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, direct.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || string(body) != want {
			t.Fatalf("body=%q error=%v want=%s", body, err, want)
		}
	}
	read("direct")
	for _, step := range []struct{ url, label string }{{first.URL, "first"}, {second.URL, "second"}, {"", "direct"}} {
		if err := manager.Update(ProxyConfig{Enabled: step.url != "", URL: step.url}); err != nil {
			t.Fatal(err)
		}
		select {
		case <-client.Changes():
		case <-time.After(time.Second):
			t.Fatal("missing proxy change")
		}
		if err := client.Refresh(); err != nil {
			t.Fatal(err)
		}
		read(step.label)
	}
	client.Close()
	client.Close()
	if _, ok := <-client.Changes(); ok {
		t.Fatal("subscription not closed")
	}
	if len(manager.subs) != 0 {
		t.Fatal("subscription leaked")
	}
	if err := client.Refresh(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("refresh after close: %v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, direct.URL, nil)
	if _, err := client.Do(req); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("request after close: %v", err)
	}
}

type heldFingerprintClient struct {
	started chan struct{}
	release chan struct{}
	closed  int
}

func (c *heldFingerprintClient) Do(req *http.Request) (*http.Response, error) {
	close(c.started)
	<-c.release
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("old response"))}, nil
}
func (c *heldFingerprintClient) CloseIdleConnections() { c.closed++ }

func TestFingerprintRefreshPreservesInFlightRequest(t *testing.T) {
	old := &heldFingerprintClient{started: make(chan struct{}), release: make(chan struct{})}
	client := &ProxiedFingerprintClient{client: old, options: FingerprintOptions{Timeout: time.Second}}
	defer client.Close()
	release := sync.OnceFunc(func() { close(old.release) })
	defer release()
	request, _ := http.NewRequest(http.MethodGet, "http://example.test", nil)
	response := make(chan *http.Response, 1)
	go func() {
		res, _ := client.Do(request)
		response <- res
	}()
	<-old.started
	if err := client.Refresh(); err != nil {
		t.Fatal(err)
	}
	if old.closed != 1 {
		t.Fatalf("old idle connections closed %d times", old.closed)
	}
	release()
	res := <-response
	if res == nil {
		t.Fatal("in-flight request lost its response")
	}
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil || string(body) != "old response" {
		t.Fatalf("old response: %q %v", body, err)
	}
}

func TestFingerprintConcurrentRefreshAndClose(t *testing.T) {
	client, err := NewProxiedFingerprintClient(nil, FingerprintOptions{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if client.Changes() != nil {
		t.Fatal("direct client should not subscribe")
	}
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() { _ = client.Refresh() })
		group.Go(client.Close)
	}
	group.Wait()
	if err := client.Refresh(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("closed client was revived: %v", err)
	}
}

func TestFingerprintRefreshFailureKeepsCurrentClient(t *testing.T) {
	old := &heldFingerprintClient{}
	client := &ProxiedFingerprintClient{
		client:  old,
		options: FingerprintOptions{Timeout: time.Second, Proxy: &url.URL{Scheme: "unsupported", Host: "127.0.0.1:1234"}},
	}
	defer client.Close()
	if err := client.Refresh(); err == nil {
		t.Fatal("expected unsupported proxy to fail")
	}
	if client.client != old || old.closed != 0 {
		t.Fatal("failed refresh discarded the current client")
	}
}
