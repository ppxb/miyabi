package netx

import (
	"net"
	"sync"

	http "github.com/bogdanfinn/fhttp"
)

// FingerprintHTTPClient is the request/connection contract shared by fingerprint users.
type FingerprintHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
	CloseIdleConnections()
}

// ProxiedFingerprintClient owns a replaceable client and its proxy subscription.
// Callers consume Changes and call Refresh before their service-specific probes.
// It starts no goroutines and leaves in-flight requests on their original client.
type ProxiedFingerprintClient struct {
	manager *ProxyManager
	changes <-chan struct{}
	options FingerprintOptions
	mu      sync.RWMutex
	client  FingerprintHTTPClient
}

func NewProxiedFingerprintClient(manager *ProxyManager, options FingerprintOptions) (*ProxiedFingerprintClient, error) {
	c := &ProxiedFingerprintClient{manager: manager, options: options}
	if manager != nil {
		c.changes = manager.Subscribe()
		options.Proxy = manager.Resolve()
	}
	client, err := NewFingerprintClient(options)
	if err != nil {
		if manager != nil {
			manager.Unsubscribe(c.changes)
		}
		return nil, err
	}
	c.client = client
	return c, nil
}

func (c *ProxiedFingerprintClient) Changes() <-chan struct{} { return c.changes }

// Refresh retains the previous client when construction fails. The lock also
// prevents a concurrent Close from being followed by installation of a new client.
func (c *ProxiedFingerprintClient) Refresh() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return net.ErrClosed
	}
	options := c.options
	if c.manager != nil {
		options.Proxy = c.manager.Resolve()
	}
	next, err := NewFingerprintClient(options)
	if err != nil {
		return err
	}
	previous := c.client
	c.client = next
	previous.CloseIdleConnections()
	return nil
}

func (c *ProxiedFingerprintClient) Do(req *http.Request) (*http.Response, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()
	if client == nil {
		return nil, net.ErrClosed
	}
	return client.Do(req)
}

func (c *ProxiedFingerprintClient) CloseIdleConnections() {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.client != nil {
		c.client.CloseIdleConnections()
	}
}

func (c *ProxiedFingerprintClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return
	}
	c.client.CloseIdleConnections()
	c.client = nil
	if c.manager != nil {
		c.manager.Unsubscribe(c.changes)
	}
}
