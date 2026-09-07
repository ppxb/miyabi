package javdb

import (
	"context"
	"errors"
	"fmt"
	"net/url"
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
	options      Options
	limiter      *rate.Limiter
	media        *resty.Client
	current      atomic.Pointer[routeState]
	routes       singleflight.Group
	routeContext context.Context
	stopRoutes   context.CancelFunc
	selectRoute  func(context.Context, string) (*routeState, error)
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
	client.selectRoute = client.selectAndInstall
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

// Initialize selects and installs an API route.
func (c *Client) Initialize(ctx context.Context) error {
	_, err := c.ensureRoute(ctx)
	return err
}

// Reselect runs a full route selection and replaces the active route on success.
func (c *Client) Reselect(ctx context.Context) (RouteStatus, error) {
	state, err := c.waitRoute(ctx, func(ctx context.Context) (*routeState, error) {
		return c.selectRoute(ctx, "")
	})
	if err != nil {
		return RouteStatus{}, err
	}
	return state.status, nil
}

// Route returns the active route, if the client has been initialized.
func (c *Client) Route() (RouteStatus, bool) {
	state := c.current.Load()
	if state == nil {
		return RouteStatus{}, false
	}
	return state.status, true
}

// Close releases idle API and image connections.
func (c *Client) Close() {
	c.stopRoutes()
	c.media.GetClient().CloseIdleConnections()
	if state := c.current.Load(); state != nil {
		state.transport.closeIdleConnections()
	}
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
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	err = state.transport.getJSON(ctx, path, params, language, destination)
	if ctx.Err() != nil || !routeFailure(err) {
		return err
	}

	state, err = c.replaceFailedRoute(ctx, state)
	if err != nil {
		return err
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	return state.transport.getJSON(ctx, path, params, language, destination)
}

func (c *Client) ensureRoute(ctx context.Context) (*routeState, error) {
	if state := c.current.Load(); state != nil {
		return state, nil
	}
	return c.waitRoute(ctx, func(ctx context.Context) (*routeState, error) {
		if state := c.current.Load(); state != nil {
			return state, nil
		}
		return c.selectRoute(ctx, c.options.CachedHost)
	})
}

func (c *Client) replaceFailedRoute(ctx context.Context, failed *routeState) (*routeState, error) {
	return c.waitRoute(ctx, func(ctx context.Context) (*routeState, error) {
		if current := c.current.Load(); current != failed {
			return current, nil
		}
		return c.selectRoute(ctx, "")
	})
}

func (c *Client) waitRoute(ctx context.Context, selectRoute func(context.Context) (*routeState, error)) (*routeState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := c.routes.DoChan("route", func() (any, error) {
		// Selection belongs to the long-lived client. A browser leaving only
		// cancels its own wait; cached, bootstrap and dynamic probes are bounded.
		shared, cancel := context.WithTimeout(c.routeContext, 3*c.options.Timeout)
		defer cancel()
		return selectRoute(shared)
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

func (c *Client) selectAndInstall(ctx context.Context, cachedHost string) (*routeState, error) {
	result, err := selectRoute(ctx, cachedHost, c.probe)
	if err != nil {
		return nil, err
	}
	transport, err := newTransport(result.Host, c.options)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		transport.closeIdleConnections()
		return nil, err
	}
	state := &routeState{
		transport: transport,
		status: RouteStatus{
			Host:    result.Host,
			Latency: result.Latency,
		},
	}
	previous := c.current.Swap(state)
	if previous != nil {
		previous.transport.closeIdleConnections()
	}
	return state, nil
}

func (c *Client) probe(ctx context.Context, host string, onStart func(time.Time)) (time.Duration, map[string]any, error) {
	transport, err := newTransport(host, c.options)
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
