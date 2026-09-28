package netx

import (
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ppxb/miyabi/internal/domain"
)

// ProxyConfig is the process-wide upstream proxy configuration. A disabled
// proxy keeps its URL so it can be enabled again without re-entering it.
type ProxyConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}

type proxyState struct {
	config ProxyConfig
	proxy  *url.URL
}

// ProxyManager provides a concurrency-safe proxy configuration and a small
// notification channel for clients that need to rebuild transports.
type ProxyManager struct {
	current atomic.Pointer[proxyState]

	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

func NewProxyManager(initial ProxyConfig) (*ProxyManager, error) {
	normalized, parsed, err := Normalize(initial)
	if err != nil {
		return nil, err
	}
	manager := &ProxyManager{subs: make(map[chan struct{}]struct{})}
	manager.current.Store(&proxyState{config: normalized, proxy: parsed})
	return manager, nil
}

// Normalize validates the URL syntax and supported schemes, strips surrounding
// whitespace, and resolves the parsed *url.URL. When Enabled is false, the URL
// is validated if present, but the returned *url.URL is always nil so clients
// connect directly without inspecting the enabled flag.
func Normalize(config ProxyConfig) (ProxyConfig, *url.URL, error) {
	config.URL = strings.TrimSpace(config.URL)
	if config.URL == "" {
		config.Enabled = false
		return config, nil, nil
	}
	parsed, err := parseProxyURL(config.URL)
	if err != nil {
		return ProxyConfig{}, nil, err
	}
	if !config.Enabled {
		return config, nil, nil
	}
	return config, parsed, nil
}

func (m *ProxyManager) Config() ProxyConfig {
	return m.current.Load().config
}

// Resolve returns a copy of the active proxy URL, or nil when the proxy is
// disabled. The returned URL can be modified by the caller safely.
func (m *ProxyManager) Resolve() *url.URL {
	return m.current.Load().resolve()
}

func (state *proxyState) resolve() *url.URL {
	if !state.config.Enabled || state.proxy == nil {
		return nil
	}
	proxy := *state.proxy
	return &proxy
}

// Apply sets the pre-normalized proxy state and broadcasts to subscribers
// if the effective proxy address or enabled state changed.
func (m *ProxyManager) Apply(normalized ProxyConfig, parsed *url.URL) {
	m.mu.Lock()
	defer m.mu.Unlock()
	previous := m.current.Load()
	sameURL := (previous.proxy == nil && parsed == nil) ||
		(previous.proxy != nil && parsed != nil && previous.proxy.String() == parsed.String())
	sameConfig := previous.config == normalized
	m.current.Store(&proxyState{config: normalized, proxy: parsed})
	if !sameConfig || !sameURL {
		m.broadcastLocked()
	}
}

// Update validates the new configuration and broadcasts to all subscribers
// if the effective proxy address or enabled state changed.
func (m *ProxyManager) Update(config ProxyConfig) error {
	normalized, parsed, err := Normalize(config)
	if err != nil {
		return err
	}
	m.Apply(normalized, parsed)
	return nil
}

func (m *ProxyManager) Subscribe() <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan struct{}, 1)
	m.subs[ch] = struct{}{}
	return ch
}

func (m *ProxyManager) Unsubscribe(ch <-chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for candidate := range m.subs {
		if candidate == ch {
			delete(m.subs, candidate)
			close(candidate)
			return
		}
	}
}

func (m *ProxyManager) broadcastLocked() {
	for ch := range m.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func parseProxyURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, nil
	}
	proxy, err := url.Parse(raw)
	if err != nil {
		return nil, invalidProxy("代理地址格式错误", err)
	}
	if proxy.Scheme == "" || proxy.Hostname() == "" {
		return nil, invalidProxy("代理地址必须包含协议（如 http://）与主机地址", nil)
	}
	proxy.Scheme = strings.ToLower(proxy.Scheme)
	switch proxy.Scheme {
	case "http", "https", "socks5":
		return proxy, nil
	}
	return nil, invalidProxy("代理协议仅支持 http://、https:// 或 socks5://", nil)
}

// invalidProxy reports a configuration validation failure. The API layer maps
// KindInvalid to 400 and shows the message verbatim.
func invalidProxy(reason string, cause error) error {
	return domain.E(domain.KindInvalid, "代理配置无效: "+reason, cause)
}
