// Package projectjav scrapes projectjav.com, a torrent index whose search page
// lists each matching movie's magnet links inline. It is used as a supplementary
// magnet source and can be switched off from settings.
package projectjav

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/time/rate"
)

const (
	defaultBaseURL = "https://projectjav.com"
	defaultTimeout = 15 * time.Second
	defaultRate    = 1
	defaultBurst   = 2
	maxBodyBytes   = 8 << 20
	userAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// Options configures a ProjectJav client.
type Options struct {
	Timeout time.Duration
	Proxy   *netx.ProxyManager
	BaseURL string

	testClient netx.FingerprintHTTPClient
}

// Client queries ProjectJav for magnets by catalogue code.
type Client struct {
	limiter   *rate.Limiter
	http      netx.FingerprintHTTPClient
	baseURL   string
	done      chan struct{}
	closeOnce sync.Once
}

// New creates a ProjectJav client using the shared fingerprint transport.
func New(options Options) (*Client, error) {
	if options.testClient != nil {
		return NewForTest(options.testClient), nil
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	base := strings.TrimRight(options.BaseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	fingerprint, err := netx.NewProxiedFingerprintClient(options.Proxy, netx.FingerprintOptions{Timeout: timeout, CookieJar: true})
	if err != nil {
		return nil, fmt.Errorf("create ProjectJav fingerprint client: %w", err)
	}
	client := &Client{
		limiter: rate.NewLimiter(rate.Limit(defaultRate), defaultBurst),
		http:    fingerprint,
		baseURL: base,
		done:    make(chan struct{}),
	}
	go client.watchProxy(fingerprint)
	return client, nil
}

// NewForTest builds a client around an injected transport with no rate limit.
func NewForTest(transport netx.FingerprintHTTPClient) *Client {
	return &Client{
		limiter: rate.NewLimiter(rate.Inf, 0),
		http:    transport,
		baseURL: defaultBaseURL,
		done:    make(chan struct{}),
	}
}

// Name identifies ProjectJav as a magnet source.
func (*Client) Name() string { return domain.MagnetSourceProjectJav }

func (c *Client) watchProxy(fingerprint *netx.ProxiedFingerprintClient) {
	fingerprint.Watch(c.done, func() {
		_ = fingerprint.Refresh()
	})
}

// Find returns the ProjectJav magnets whose search result matches the code.
func (c *Client) Find(ctx context.Context, ref domain.MovieRef) ([]domain.Magnet, error) {
	code := codeid.Normalize(ref.Code)
	if code == "" {
		return nil, nil
	}
	body, err := c.search(ctx, code)
	if err != nil {
		return nil, err
	}
	return parseSearch(body, code), nil
}

func (c *Client) search(ctx context.Context, code string) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	endpoint := c.baseURL + "/?searchTerm=" + url.QueryEscape(code)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create ProjectJav request: %w", err)
	}
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8")

	response, err := c.http.Do(request)
	if err != nil {
		return nil, domain.E(domain.KindUpstream, "ProjectJav 请求失败", err)
	}
	defer response.Body.Close()
	if response.StatusCode == 403 || response.StatusCode == 503 {
		_, _ = io.CopyN(io.Discard, response.Body, 4096)
		return nil, domain.Blocked(fmt.Sprintf("ProjectJav 返回状态 %d", response.StatusCode))
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.CopyN(io.Discard, response.Body, 4096)
		return nil, domain.E(domain.KindUpstream, fmt.Sprintf("ProjectJav 返回状态 %d", response.StatusCode), nil)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes))
	if err != nil {
		return nil, domain.E(domain.KindUpstream, "读取 ProjectJav 响应失败", err)
	}
	if netx.IsCloudflareInterstitial(body) {
		return nil, domain.Blocked("ProjectJav 遇到 Cloudflare 挑战")
	}
	return body, nil
}

// Close releases the transport and stops proxy watching. Safe to call twice.
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		close(c.done)
		if fingerprint, ok := c.http.(*netx.ProxiedFingerprintClient); ok {
			fingerprint.Close()
		}
	})
}
