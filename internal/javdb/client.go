package javdb

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"
)

type routeState struct {
	transport jsonTransport
	status    RouteStatus
}

type jsonTransport interface {
	getJSON(context.Context, string, url.Values, string, any) error
	closeIdleConnections()
}

// Client is the anonymous JavDB App API client.
type Client struct {
	options       Options
	limiter       *rate.Limiter
	cooldownMu    sync.Mutex
	cooldownUntil time.Time
	media         *resty.Client
	current       atomic.Pointer[routeState]
	lastProbe     atomic.Pointer[[]RouteCandidate]
	routes        singleflight.Group
	selectionMu   sync.Mutex
	routeContext  context.Context
	stopRoutes    context.CancelFunc
	selector      func(context.Context, routeSelection) (*routeState, error)
	proxyChanges  <-chan struct{}
}

func New(options Options) (*Client, error) {
	if options.DeviceUUID == "" {
		return nil, errors.New("JavDB device UUID is required")
	}
	if _, err := uuid.Parse(options.DeviceUUID); err != nil {
		return nil, fmt.Errorf("parse JavDB device UUID: %w", err)
	}
	if options.Timeout < 0 {
		return nil, errors.New("JavDB timeout must not be negative")
	}
	if options.Timeout == 0 {
		options.Timeout = defaultTimeout
	}
	if options.RequestsPerSecond == 0 {
		options.RequestsPerSecond = defaultRate
	}
	if options.Burst == 0 {
		options.Burst = defaultBurst
	}
	if options.RequestsPerSecond < 0 || options.Burst < 0 {
		return nil, errors.New("JavDB rate limit must not be negative")
	}

	routeContext, stopRoutes := context.WithCancel(context.Background())
	client := &Client{
		options:      options,
		limiter:      rate.NewLimiter(rate.Limit(options.RequestsPerSecond), options.Burst),
		media:        newMediaClient(options),
		routeContext: routeContext,
		stopRoutes:   stopRoutes,
	}
	client.selector = client.selectAndInstall
	if options.Proxy != nil {
		client.proxyChanges = options.Proxy.Subscribe()
		go client.watchProxy()
	}
	if options.CachedHost != "" {
		// Restore the last successful selection without probing. The first GET
		// uses this transport and retains the normal failure recovery path.
		hosts := append(slices.Clone(bootstrapHosts), options.CachedHost)
		known := map[string]probeResult{
			options.CachedHost: {latency: options.CachedLatency},
		}
		if _, err := client.installRoute(routeContext, RouteStatus{
			Host: options.CachedHost, Latency: options.CachedLatency, Manual: options.ManualRoute,
			Candidates: routeCandidates(hosts, known),
		}); err != nil {
			client.Close()
			return nil, fmt.Errorf("restore cached JavDB route: %w", err)
		}
	}
	return client, nil
}

// NewDeviceUUID creates a device identifier to persist in settings.
func NewDeviceUUID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("create JavDB device UUID: %w", err)
	}
	return id.String(), nil
}

// Initialize selects an API route when no cached or active route is available.
func (c *Client) Initialize(ctx context.Context) error {
	_, err := c.ensureRoute(ctx)
	return err
}

// Reselect measures every candidate and replaces the active route on success.
func (c *Client) Reselect(ctx context.Context) (RouteStatus, error) {
	state, err := c.waitRoute(ctx, "probe-all", func(ctx context.Context) (*routeState, error) {
		return c.selector(ctx, routeSelection{full: true, hosts: c.routeHosts()})
	})
	if err != nil {
		return RouteStatus{}, err
	}
	return state.status, nil
}

// SelectRoute verifies a known candidate before changing the active route.
// Connection failures still trigger automatic selection on subsequent requests.
func (c *Client) SelectRoute(ctx context.Context, rawHost string) (RouteStatus, error) {
	c.selectionMu.Lock()
	defer c.selectionMu.Unlock()
	if err := ctx.Err(); err != nil {
		return RouteStatus{}, err
	}
	host, err := normalizeHost(rawHost)
	if err != nil {
		return RouteStatus{}, err
	}
	current, _ := c.Route()
	index := slices.IndexFunc(current.Candidates, func(candidate RouteCandidate) bool { return candidate.Host == host })
	if index < 0 {
		return RouteStatus{}, fmt.Errorf("unknown JavDB route %q", host)
	}
	latency, _, err := c.probe(ctx, host, nil)
	candidates := slices.Clone(current.Candidates)
	candidates[index] = RouteCandidate{Host: host, Latency: latency, Status: RouteAvailable}
	if err != nil {
		if ctx.Err() == nil {
			candidates[index].Status = RouteUnavailable
			c.lastProbe.Store(&candidates)
		}
		return RouteStatus{}, err
	}
	state, err := c.installRoute(ctx, RouteStatus{
		Host: host, Latency: latency, Manual: true, Candidates: candidates,
	})
	if err != nil {
		return RouteStatus{}, err
	}
	return state.status, nil
}

// Route returns the active route, if the client has been initialized.
func (c *Client) Route() (RouteStatus, bool) {
	state := c.current.Load()
	var status RouteStatus
	if state != nil {
		status = state.status
	}
	if candidates := c.lastProbe.Load(); candidates != nil {
		status.Candidates = *candidates
	} else if state == nil {
		hosts := slices.Clone(bootstrapHosts)
		if c.options.CachedHost != "" {
			hosts = append(hosts, c.options.CachedHost)
		}
		status.Candidates = routeCandidates(hosts, nil)
	}
	return status, state != nil
}

func (c *Client) routeHosts() []string {
	status, _ := c.Route()
	hosts := make([]string, 0, len(status.Candidates)+1)
	for _, candidate := range status.Candidates {
		hosts = append(hosts, candidate.Host)
	}
	if status.Host != "" {
		hosts = append(hosts, status.Host)
	}
	return hosts
}

// Close releases idle API and image connections.
func (c *Client) Close() {
	c.stopRoutes()
	if c.options.Proxy != nil && c.proxyChanges != nil {
		c.options.Proxy.Unsubscribe(c.proxyChanges)
	}
	c.media.GetClient().CloseIdleConnections()
	if state := c.current.Load(); state != nil {
		state.transport.closeIdleConnections()
	}
}

// proxyURL returns the proxy to bind into a new transport, or nil for direct.
func (c *Client) proxyURL() *url.URL {
	if c.options.Proxy == nil {
		return nil
	}
	return c.options.Proxy.Resolve()
}

// watchProxy rebuilds the active transport whenever the proxy changes and,
// for automatically selected routes, re-measures candidates through it.
func (c *Client) watchProxy() {
	for {
		select {
		case <-c.routeContext.Done():
			return
		case _, ok := <-c.proxyChanges:
			if !ok {
				return
			}
			state, err := c.reinstall()
			if err != nil {
				slog.WarnContext(c.routeContext, "JavDB transport keeps previous proxy after change", "error", err)
				continue
			}
			if state != nil && !state.status.Manual {
				go func() { _, _ = c.Reselect(c.routeContext) }()
			}
		}
	}
}

// reinstall swaps the active route's transport for one built with the current
// proxy. It returns nil when no route is installed yet.
func (c *Client) reinstall() (*routeState, error) {
	c.selectionMu.Lock()
	defer c.selectionMu.Unlock()
	previous := c.current.Load()
	if previous == nil {
		return nil, nil
	}
	transport, err := newTransport(previous.status.Host, c.proxyURL(), c.options)
	if err != nil {
		return nil, err
	}
	state := &routeState{transport: transport, status: previous.status}
	c.current.Store(state)
	previous.transport.closeIdleConnections()
	return state, nil
}

func (c *Client) getJSON(
	ctx context.Context,
	path string,
	params url.Values,
	language string,
	destination any,
) error {
	state, err := c.ensureRoute(ctx)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(maxRateLimitWait)
	failover := false
	for retries := 0; ; {
		if err := c.waitRequest(ctx, deadline); err != nil {
			return err
		}
		if current := c.current.Load(); current != nil {
			state = current
		}
		err = state.transport.getJSON(ctx, path, params, language, destination)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var response *HTTPError
		if errors.As(err, &response) && response.StatusCode == 429 {
			delay := response.RetryAfter
			if delay <= 0 {
				base := time.Second << retries
				delay = base + rand.N(base/2)
			}
			c.deferRequests(delay)
			if retries == maxRateLimitRetries {
				return err
			}
			retries++
			continue
		}
		if !routeFailure(err) || failover {
			return err
		}
		state, err = c.replaceFailedRoute(ctx, state)
		if err != nil {
			return err
		}
		failover = true
	}
}

func (c *Client) ensureRoute(ctx context.Context) (*routeState, error) {
	if state := c.current.Load(); state != nil {
		return state, nil
	}
	return c.waitRoute(ctx, "initialize", func(ctx context.Context) (*routeState, error) {
		if state := c.current.Load(); state != nil {
			return state, nil
		}
		options := routeSelection{full: true, hosts: c.routeHosts()}
		if c.options.ManualRoute {
			options.preferredHost = c.options.CachedHost
		}
		return c.selector(ctx, options)
	})
}

func (c *Client) replaceFailedRoute(ctx context.Context, failed *routeState) (*routeState, error) {
	return c.waitRoute(ctx, "recover", func(ctx context.Context) (*routeState, error) {
		if current := c.current.Load(); current != failed {
			return current, nil
		}
		return c.selector(ctx, routeSelection{hosts: c.routeHosts()})
	})
}

func (c *Client) waitRoute(ctx context.Context, key string, selectFn func(context.Context) (*routeState, error)) (*routeState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := c.routes.DoChan(key, func() (any, error) {
		c.selectionMu.Lock()
		defer c.selectionMu.Unlock()
		// Selection belongs to the long-lived client. A browser leaving only
		// cancels its own wait; bootstrap and dynamic probes are bounded.
		shared, cancel := context.WithTimeout(c.routeContext, 3*c.options.Timeout)
		defer cancel()
		return selectFn(shared)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case selected := <-result:
		if selected.Err != nil {
			return nil, selected.Err
		}
		return selected.Val.(*routeState), nil
	}
}

func (c *Client) selectAndInstall(ctx context.Context, options routeSelection) (*routeState, error) {
	result, err := selectRoute(ctx, options, c.probe)
	// Publish completed measurements even if every candidate failed.
	if result.Candidates != nil {
		c.lastProbe.Store(&result.Candidates)
	}
	if err != nil {
		return nil, err
	}
	return c.installRoute(ctx, RouteStatus{
		Host: result.Host, Latency: result.Latency,
		Manual:     result.Manual,
		Candidates: result.Candidates,
	})
}

func (c *Client) installRoute(ctx context.Context, status RouteStatus) (*routeState, error) {
	transport, err := newTransport(status.Host, c.proxyURL(), c.options)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		transport.closeIdleConnections()
		return nil, err
	}
	state := &routeState{
		transport: transport,
		status:    status,
	}
	previous := c.current.Swap(state)
	c.lastProbe.Store(&status.Candidates)
	if previous != nil {
		previous.transport.closeIdleConnections()
	}
	return state, nil
}

func (c *Client) probe(ctx context.Context, host string, onStart func(time.Time)) (time.Duration, map[string]any, error) {
	return probeHost(ctx, host, c.proxyURL(), c.options, onStart)
}

func probeHost(ctx context.Context, host string, proxy *url.URL, options Options, onStart func(time.Time)) (time.Duration, map[string]any, error) {
	transport, err := newTransport(host, proxy, options)
	if err != nil {
		return 0, nil, err
	}
	defer transport.closeIdleConnections()

	started := time.Now()
	if onStart != nil {
		onStart(started)
	}
	var startup map[string]any
	if err := transport.getJSON(ctx, "/api/v1/startup", nil, defaultLanguage, &startup); err != nil {
		return 0, nil, err
	}
	return time.Since(started), startup, nil
}

func routeFailure(err error) bool {
	if err == nil {
		return false
	}
	var network *networkError
	if errors.As(err, &network) {
		return true
	}
	var response *HTTPError
	if errors.As(err, &response) {
		switch response.StatusCode {
		case 502, 503, 504:
			return true
		}
	}
	return false
}

// Probe measures the bootstrap JavDB route through the given proxy, or
// directly when proxy is nil.
func Probe(ctx context.Context, proxy *url.URL, timeout time.Duration) (time.Duration, error) {
	latency, _, err := probeHost(ctx, bootstrapHosts[0], proxy, Options{Timeout: timeout}, nil)
	return latency, err
}
