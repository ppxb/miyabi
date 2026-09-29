package netx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// MaxSafeDownloadBytes is the maximum allowed response size for untrusted downloads (5MB).
	MaxSafeDownloadBytes int64 = 5 * 1024 * 1024
	// MaxRedirects is the maximum number of redirects allowed for safe downloads.
	MaxRedirects = 5
)

// IsPrivateOrLoopbackIP checks if an IP belongs to private, loopback, link-local,
// carrier-grade NAT, or unspecified networks.
func IsPrivateOrLoopbackIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	// Standard checks
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}

	// Check IPv4 specific ranges that might not be covered
	if ip4 := ip.To4(); ip4 != nil {
		// 0.0.0.0/8 (Current network)
		if ip4[0] == 0 {
			return true
		}
		// 100.64.0.0/10 (Shared Address Space / CGNAT)
		if ip4[0] == 100 && (ip4[1]&0xc0) == 64 {
			return true
		}
		// 192.0.0.0/24 (IETF Protocol Assignments)
		if ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 0 {
			return true
		}
		// 198.18.0.0/15 (Benchmarking)
		if ip4[0] == 198 && (ip4[1]&0xfe) == 18 {
			return true
		}
		// 240.0.0.0/4 (Reserved)
		if ip4[0] >= 240 {
			return true
		}
		// 255.255.255.255 (Broadcast)
		if ip4[0] == 255 && ip4[1] == 255 && ip4[2] == 255 && ip4[3] == 255 {
			return true
		}
	}

	return false
}

// ValidateSafeURL checks that the URL scheme is http/https and does not resolve to private/loopback IPs.
func ValidateSafeURL(ctx context.Context, rawURL string, allowLoopback bool) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme: %s (only http and https allowed)", parsed.Scheme)
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return nil, errors.New("empty hostname in url")
	}

	if strings.EqualFold(hostname, "localhost") {
		if !allowLoopback {
			return nil, errors.New("access to localhost is prohibited")
		}
		return parsed, nil
	}

	// If hostname is directly an IP literal
	if ip := net.ParseIP(hostname); ip != nil {
		if IsPrivateOrLoopbackIP(ip) {
			if allowLoopback && ip.IsLoopback() {
				return parsed, nil
			}
			return nil, fmt.Errorf("access to private/loopback IP %s is prohibited", ip)
		}
		return parsed, nil
	}

	// Resolve hostname to IPs and ensure none are private/loopback
	resolver := net.DefaultResolver
	ips, err := resolver.LookupIP(ctx, "ip", hostname)
	if err != nil {
		return nil, fmt.Errorf("resolve hostname %s: %w", hostname, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no IP resolved for %s", hostname)
	}

	for _, ip := range ips {
		if IsPrivateOrLoopbackIP(ip) {
			if allowLoopback && ip.IsLoopback() {
				continue
			}
			return nil, fmt.Errorf("domain %s resolved to private/loopback IP %s", hostname, ip)
		}
	}

	return parsed, nil
}

// NewSafeTransport creates an http.Transport with SSRF protection at dial time.
func NewSafeTransport(proxyManager *ProxyManager) *http.Transport {
	transport := NewTransport(proxyManager)

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	// Enforce SSRF protection at TCP dial time to prevent DNS rebinding attacks
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		// When traffic is routed through a proxy, addr is the proxy server itself
		// (e.g. 127.0.0.1:7890 or a LAN proxy). Direct SSRF checks apply strictly
		// to direct connections, as the proxy handles upstream resolution.
		if proxyManager != nil && proxyManager.Resolve() != nil {
			return dialer.DialContext(ctx, network, addr)
		}

		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		if strings.EqualFold(host, "localhost") {
			return nil, errors.New("access to localhost is prohibited")
		}

		if ip := net.ParseIP(host); ip != nil {
			if IsPrivateOrLoopbackIP(ip) {
				return nil, fmt.Errorf("access to private/loopback IP %s is prohibited", ip)
			}
			return dialer.DialContext(ctx, network, addr)
		}

		// Resolve domain and check every resolved IP
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("dns resolve error: %w", err)
		}

		var firstErr error
		for _, ip := range ips {
			if IsPrivateOrLoopbackIP(ip) {
				return nil, fmt.Errorf("access to private/loopback IP %s is prohibited", ip)
			}
		}

		// Connect to the verified IP directly
		for _, ip := range ips {
			target := net.JoinHostPort(ip.String(), port)
			conn, err := dialer.DialContext(ctx, network, target)
			if err == nil {
				return conn, nil
			}
			if firstErr == nil {
				firstErr = err
			}
		}

		return nil, firstErr
	}

	return transport
}

// NewSafeDownloadClient applies SSRF protection, redirect checks and a timeout.
// SafeDownload enforces the response body size limit.
func NewSafeDownloadClient(proxyManager *ProxyManager, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	transport := NewSafeTransport(proxyManager)

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= MaxRedirects {
				return fmt.Errorf("stopped after %d redirects", MaxRedirects)
			}
			// Re-validate target URL on every redirect hop
			if _, err := ValidateSafeURL(req.Context(), req.URL.String(), false); err != nil {
				return fmt.Errorf("redirect blocked: %w", err)
			}
			return nil
		},
	}
}

// DownloadOption configures SafeDownload behavior.
type DownloadOption func(*downloadOptions)

type downloadOptions struct {
	maxBytes      int64
	allowLoopback bool
}

// WithMaxBytes sets the maximum allowed download bytes.
func WithMaxBytes(maxBytes int64) DownloadOption {
	return func(o *downloadOptions) {
		o.maxBytes = maxBytes
	}
}

// WithAllowLoopback configures whether loopback is allowed (used strictly in test environments).
func WithAllowLoopback(allow bool) DownloadOption {
	return func(o *downloadOptions) {
		o.allowLoopback = allow
	}
}

// SafeDownload performs a safe HTTP GET download, verifying URL, enforcing SSRF checks,
// and reading at most maxBytes (defaulting to MaxSafeDownloadBytes if <= 0).
func SafeDownload(ctx context.Context, client *http.Client, targetURL string, opts ...DownloadOption) ([]byte, error) {
	cfg := downloadOptions{
		maxBytes:      MaxSafeDownloadBytes,
		allowLoopback: false,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	if _, err := ValidateSafeURL(ctx, targetURL, cfg.allowLoopback); err != nil {
		return nil, fmt.Errorf("safe url check failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download status code %d", resp.StatusCode)
	}

	// Limit reader to maxBytes + 1 to detect overflow
	limitedReader := io.LimitReader(resp.Body, cfg.maxBytes+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("reading response body failed: %w", err)
	}

	if int64(len(body)) > cfg.maxBytes {
		return nil, fmt.Errorf("download exceeded maximum allowed size (%d bytes)", cfg.maxBytes)
	}

	return body, nil
}
