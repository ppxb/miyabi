package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHealthcheckUsesLoopbackForWildcardListeners(t *testing.T) {
	for _, family := range []struct {
		name    string
		network string
		address string
		hosts   []string
	}{
		{"IPv4", "tcp4", "127.0.0.1:0", []string{"", "0.0.0.0", "127.0.0.1"}},
		{"IPv6", "tcp6", "[::1]:0", []string{"::", "::1"}},
	} {
		t.Run(family.name, func(t *testing.T) {
			listener, err := net.Listen(family.network, family.address)
			if err != nil {
				if family.network == "tcp6" {
					t.Skipf("IPv6 loopback unavailable: %v", err)
				}
				t.Fatal(err)
			}
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/health" {
					t.Errorf("unexpected probe path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			server.Listener.Close()
			server.Listener = listener
			server.Start()
			t.Cleanup(server.Close)
			_, port, err := net.SplitHostPort(listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			for _, host := range family.hosts {
				if err := checkHealth(net.JoinHostPort(host, port)); err != nil {
					t.Errorf("probe %q: %v", host, err)
				}
			}
		})
	}
}

func TestHealthcheckRejectsUnhealthyResponsesAndRedirects(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/redirected" {
					t.Error("health probe followed a redirect")
					w.WriteHeader(http.StatusOK)
					return
				}
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(status)
			}))
			defer server.Close()
			if err := checkHealth(server.Listener.Addr().String()); err == nil {
				t.Fatalf("accepted HTTP %d", status)
			}
		})
	}
}

func TestHealthcheckTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	if err := checkHealth(server.Listener.Addr().String()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected probe timeout, got %v", err)
	}
}

func TestHealthcheckModeUsesConfigWithoutStartingServices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	dataDir := filepath.Join(t.TempDir(), "unused-data")
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte("listen = ':1'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MIYABI_LISTEN", server.Listener.Addr().String())
	t.Setenv("MIYABI_DATA_DIR", dataDir)
	t.Setenv("MIYABI_LOG_LEVEL", "info")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	if err := run([]string{"healthcheck", "-config", configPath}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dataDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("health check initialized application data: %v", err)
	}
}
