package javbus

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/magnet"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/time/rate"
)

// JavBus has a single public endpoint; there are no mirrors to manage.
const (
	baseURL        = "https://www.javbus.com"
	defaultTimeout = 15 * time.Second
	probeInterval  = 2 * time.Minute
	defaultRate    = 1
	defaultBurst   = 2
	userAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

type HTTPClient = netx.FingerprintHTTPClient

// Options configures a JavBus client.
type Options struct {
	Timeout time.Duration
	Proxy   *netx.ProxyManager

	testClient HTTPClient
}

// Client accesses JavBus for movie magnets and metadata.
type Client struct {
	limiter *rate.Limiter

	available atomic.Bool

	client      HTTPClient
	fingerprint *netx.ProxiedFingerprintClient
	isTest      bool

	ctx    context.Context
	cancel context.CancelFunc
}

type defaultTestHTTPClient struct{}

func (defaultTestHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 404,
		Body:       io.NopCloser(strings.NewReader("not found")),
		Header:     make(http.Header),
	}, nil
}

func (defaultTestHTTPClient) CloseIdleConnections() {}

// New creates a JavBus client.
func New(options Options) (*Client, error) {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	ctx, cancel := context.WithCancel(context.Background())
	client := &Client{
		limiter: rate.NewLimiter(rate.Limit(defaultRate), defaultBurst),
		ctx:     ctx,
		cancel:  cancel,
	}

	if options.testClient != nil {
		client.client = options.testClient
		client.isTest = true
		client.available.Store(true)
		return client, nil
	}

	initialClient, err := netx.NewProxiedFingerprintClient(options.Proxy, netx.FingerprintOptions{Timeout: timeout, CookieJar: true})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create JavBus fingerprint client: %w", err)
	}
	client.client = initialClient
	client.fingerprint = initialClient

	go client.runHealthLoop()

	return client, nil
}

// NewForTest creates a JavBus client for testing with a specified availability.
func NewForTest(available bool, testClients ...HTTPClient) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	var cl HTTPClient = defaultTestHTTPClient{}
	if len(testClients) > 0 && testClients[0] != nil {
		cl = testClients[0]
	}
	client := &Client{
		isTest:  true,
		client:  cl,
		limiter: rate.NewLimiter(rate.Inf, 0),
		ctx:     ctx,
		cancel:  cancel,
	}
	client.available.Store(available)
	return client
}

// SetAvailableForTest sets the client's availability for testing.
func (c *Client) SetAvailableForTest(available bool) {
	c.available.Store(available)
}

// Name identifies the magnet source.
func (c *Client) Name() string {
	return domain.MagnetSourceJavBus
}

// Available reports whether JavBus is currently reachable.
func (c *Client) Available() bool {
	return c.available.Load()
}

func (c *Client) setAvailable(next bool) {
	prev := c.available.Swap(next)
	if prev != next {
		if next {
			slog.InfoContext(c.ctx, "JavBus source is reachable, magnet aggregation enabled")
		} else {
			slog.WarnContext(c.ctx, "JavBus source is unreachable, magnet aggregation degraded")
		}
	}
}

func (c *Client) probe() bool {
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Cookie", "dv=1")

	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, 512)
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

// Find retrieves magnets for the specified movie reference.
func (c *Client) Find(ctx context.Context, ref domain.MovieRef) ([]domain.Magnet, error) {
	if !c.available.Load() {
		return nil, magnet.ErrSkipped
	}

	code := strings.TrimSpace(ref.Code)
	if code == "" {
		return nil, nil
	}

	// Skip categories JavBus does not curate or formats with irregular quality.
	if ref.Zone == domain.ZoneWestern || ref.Zone == domain.ZoneAnime || ref.Zone == domain.ZoneFC2 {
		return nil, nil
	}
	if strings.HasPrefix(strings.ToUpper(code), "FC2") {
		return nil, nil
	}

	gid, uc, img, err := c.ensureDetailParams(ctx, code)
	if err != nil {
		return nil, err
	}
	if gid == "" {
		// Movie was not found on JavBus.
		return nil, nil
	}

	return c.fetchMagnets(ctx, code, gid, uc, img)
}

func (c *Client) ensureDetailParams(ctx context.Context, code string) (gid, uc, img string, err error) {
	body, err := c.MoviePage(ctx, code)
	if err != nil || body == "" {
		return "", "", "", err
	}
	params, err := extractDetailParams(body)
	return params.GID, params.UC, params.Img, err
}

// MoviePage retrieves the verified page shared by metadata and magnet parsing.
func (c *Client) MoviePage(ctx context.Context, code string) (string, error) {
	detailURL := fmt.Sprintf("%s/%s?existmag=all", baseURL, url.PathEscape(code))
	resp, err := c.get(ctx, detailURL, "detail", http.Header{
		"Accept": {"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 301 || resp.StatusCode == 302 {
		location := resp.Header.Get("Location")
		if strings.Contains(location, "driver-verify") {
			return "", domain.E(domain.KindUpstream, "JavBus driver verify required", nil)
		}
		return "", domain.E(domain.KindUpstream, fmt.Sprintf("JavBus unexpected redirect to %s", location), nil)
	}

	body, err := readResponse(resp.Body, "detail")
	if err != nil {
		return "", err
	}

	if resp.StatusCode == 404 || isNotFoundPage(body) {
		return "", nil
	}

	if err := c.validateResponse(resp.StatusCode, body); err != nil {
		return "", err
	}

	return body, nil
}

func (c *Client) fetchMagnets(ctx context.Context, code, gid, uc, img string) ([]domain.Magnet, error) {
	floor := rand.IntN(1000) + 1
	ajaxURL := fmt.Sprintf("%s/ajax/uncledatoolsbyajax.php?gid=%s&lang=zh&img=%s&uc=%s&floor=%d",
		baseURL, url.QueryEscape(gid), url.QueryEscape(img), url.QueryEscape(uc), floor)

	resp, err := c.get(ctx, ajaxURL, "ajax magnets", http.Header{
		"Referer":          {fmt.Sprintf("%s/%s", baseURL, url.PathEscape(code))},
		"X-Requested-With": {"XMLHttpRequest"},
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 301 || resp.StatusCode == 302 {
		return nil, domain.E(domain.KindUpstream, "JavBus driver verify required", nil)
	}

	body, err := readResponse(resp.Body, "ajax magnets")
	if err != nil {
		return nil, err
	}
	if err := c.validateResponse(resp.StatusCode, body); err != nil {
		return nil, err
	}

	return parseMagnetsHTML(body)
}

// get shares transport handling; callers own response bodies and endpoint semantics.
func (c *Client) get(ctx context.Context, requestURL, operation string, headers http.Header) (*http.Response, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	for key, values := range headers {
		req.Header[key] = values
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Cookie", "dv=1; existmag=all")

	resp, err := c.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !c.isTest {
			c.setAvailable(false)
		}
		return nil, domain.E(domain.KindUpstream, "JavBus "+operation+" request failed", err)
	}
	return resp, nil
}

func readResponse(body io.Reader, operation string) (string, error) {
	data, err := io.ReadAll(io.LimitReader(body, 2<<20))
	if err != nil {
		return "", domain.E(domain.KindUpstream, "read JavBus "+operation+" response", err)
	}
	return string(data), nil
}

func (c *Client) validateResponse(status int, body string) error {
	if isCloudflareChallenge(body) || status == 403 || status == 503 {
		if !c.isTest {
			c.setAvailable(false)
		}
		return domain.E(domain.KindUpstream, "JavBus Cloudflare challenge encountered", nil)
	}
	if isDriverVerify(body) {
		return domain.E(domain.KindUpstream, "JavBus driver verify required", nil)
	}
	if status < 200 || status >= 300 {
		return domain.E(domain.KindUpstream, fmt.Sprintf("JavBus returned unexpected status %d", status), nil)
	}
	return nil
}

func (c *Client) runHealthLoop() {
	c.setAvailable(c.probe())

	ticker := time.NewTicker(probeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.setAvailable(c.probe())
		case _, ok := <-c.fingerprint.Changes():
			if !ok {
				return
			}
			if err := c.fingerprint.Refresh(); err != nil {
				slog.WarnContext(c.ctx, "JavBus transport keeps previous proxy after change", "error", err)
				continue
			}
			c.setAvailable(c.probe())
		}
	}
}

// Close releases network resources and proxy subscription.
func (c *Client) Close() {
	c.cancel()
	if c.fingerprint != nil {
		c.fingerprint.Close()
	} else {
		c.client.CloseIdleConnections()
	}
}
