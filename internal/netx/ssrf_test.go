package netx

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIsPrivateOrLoopbackIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.2", true},
		{"::1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.1.1", true},
		{"169.254.1.1", true},
		{"100.64.0.1", true},
		{"0.0.0.0", true},
		{"::", true},
		{"255.255.255.255", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"104.21.23.45", false},
	}

	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if ip == nil {
			t.Fatalf("failed to parse %s", tt.ip)
		}
		if got := IsPrivateOrLoopbackIP(ip); got != tt.expected {
			t.Errorf("IP %s: got %v, want %v", tt.ip, got, tt.expected)
		}
	}
}

func TestValidateSafeURL(t *testing.T) {
	ctx := context.Background()

	// Prohibited URLs
	prohibited := []string{
		"http://127.0.0.1/secret",
		"http://localhost:8080/metrics",
		"http://192.168.1.1/admin",
		"http://10.10.10.10/token",
		"file:///etc/passwd",
		"ftp://example.com/file",
	}

	for _, u := range prohibited {
		_, err := ValidateSafeURL(ctx, u)
		if err == nil {
			t.Errorf("URL %s should be rejected", u)
		}
	}

	// A public literal avoids depending on external DNS.
	_, err := ValidateSafeURL(ctx, "https://93.184.216.34/subtitle.srt")
	if err != nil {
		t.Fatalf("public URL rejected: %v", err)
	}
}

func TestSafeDownload_BlocksLoopback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("sensitive internal data"))
	}))
	defer server.Close()

	client := NewSafeDownloadClient(nil, 2*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := SafeDownload(ctx, client, server.URL)
	if err == nil {
		t.Fatalf("SafeDownload must block loopback test server")
	}
	if !strings.Contains(err.Error(), "prohibited") && !strings.Contains(err.Error(), "blocked") {
		t.Errorf("expected prohibition error, got: %v", err)
	}
}

func TestSafeTransportBlocksPrivateAddressesAtDialTime(t *testing.T) {
	transport := NewSafeTransport(nil)
	defer transport.CloseIdleConnections()
	for _, address := range []string{"127.0.0.1:80", "[::1]:80", "localhost:80", "10.0.0.1:80"} {
		t.Run(address, func(t *testing.T) {
			conn, err := transport.DialContext(t.Context(), "tcp", address)
			if conn != nil {
				conn.Close()
			}
			if err == nil || !strings.Contains(err.Error(), "prohibited") {
				t.Fatalf("expected dial-time rejection, got %v", err)
			}
		})
	}
}

func TestSafeDownload_PermitsLocalProxy(t *testing.T) {
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("proxied subtitle content"))
	}))
	defer proxyServer.Close()

	proxyManager, err := NewProxyManager(ProxyConfig{Enabled: true, URL: proxyServer.URL})
	if err != nil {
		t.Fatal(err)
	}

	client := NewSafeDownloadClient(proxyManager, 2*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Downloading a public IP target via the local 127.0.0.1 proxy must succeed
	body, err := SafeDownload(ctx, client, "http://93.184.216.34/sub.srt")
	if err != nil {
		t.Fatalf("SafeDownload through local proxy should succeed, got: %v", err)
	}
	if string(body) != "proxied subtitle content" {
		t.Fatalf("expected proxied subtitle content, got: %s", string(body))
	}
}

func TestSafeDownloadLimitsAndRedirects(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/private":
			w.Header().Set("Location", "http://127.0.0.1/private")
			w.WriteHeader(http.StatusFound)
		case "/loop":
			w.Header().Set("Location", "http://93.184.216.34/loop")
			w.WriteHeader(http.StatusFound)
		case "/redirect":
			w.Header().Set("Location", "http://93.184.216.34/body")
			w.WriteHeader(http.StatusFound)
		case "/limit":
			_, _ = w.Write([]byte(strings.Repeat("x", int(MaxSafeDownloadBytes))))
		case "/overflow":
			_, _ = w.Write([]byte(strings.Repeat("x", int(MaxSafeDownloadBytes)+1)))
		case "/error":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			_, _ = w.Write([]byte("12345"))
		}
	}))
	defer proxy.Close()
	manager, err := NewProxyManager(ProxyConfig{Enabled: true, URL: proxy.URL})
	if err != nil {
		t.Fatal(err)
	}
	client := NewSafeDownloadClient(manager, time.Second)
	defer client.CloseIdleConnections()
	for _, tc := range []struct {
		path    string
		size    int
		failure string
	}{
		{"/body", 5, ""},
		{"/limit", int(MaxSafeDownloadBytes), ""},
		{"/overflow", 0, "exceeded"},
		{"/redirect", 5, ""},
		{"/private", 5, "redirect blocked"},
		{"/loop", 5, "stopped after"},
		{"/error", 5, "status code 503"},
	} {
		t.Run(tc.path+tc.failure, func(t *testing.T) {
			body, err := SafeDownload(t.Context(), client, "http://93.184.216.34"+tc.path)
			if tc.failure == "" {
				if err != nil || len(body) != tc.size {
					t.Fatalf("download bytes=%d: %v", len(body), err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.failure) {
				t.Fatalf("expected %q, got %v", tc.failure, err)
			}
		})
	}
}
