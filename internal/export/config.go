package export

import (
	"context"
	"sync"
	"sync/atomic"
)

// Config holds the unified configuration for exporting media and STRM files to Emby.
type Config struct {
	EmbyDir   string
	PublicURL string
	STRMToken string
}

// Manager coordinates atomic access to the export configuration and serializes STRM rewrites.
type Manager struct {
	cfg       atomic.Pointer[Config]
	rewriteMu sync.Mutex
}

// NewManager creates an export Manager initialized with the given Config.
func NewManager(initial Config) *Manager {
	m := &Manager{}
	m.cfg.Store(&initial)
	return m
}

// Config returns a point-in-time copy of the current export configuration.
func (m *Manager) Config() Config {
	if m == nil {
		return Config{}
	}
	if c := m.cfg.Load(); c != nil {
		return *c
	}
	return Config{}
}

// Set atomically replaces the current export configuration.
func (m *Manager) Set(cfg Config) {
	if m == nil {
		return
	}
	m.cfg.Store(&cfg)
}

// Update atomically mutates the configuration using a transform function.
func (m *Manager) Update(fn func(old Config) Config) Config {
	if m == nil {
		return Config{}
	}
	for {
		old := m.cfg.Load()
		var oldVal Config
		if old != nil {
			oldVal = *old
		}
		newVal := fn(oldVal)
		if m.cfg.CompareAndSwap(old, &newVal) {
			return newVal
		}
	}
}

// RewriteSTRM executes a serialized STRM rewrite using the current or specified configuration.
func (m *Manager) RewriteSTRM(ctx context.Context, embyDir, publicURL, strmToken string) (int, error) {
	if m != nil {
		m.rewriteMu.Lock()
		defer m.rewriteMu.Unlock()
	}
	return RewriteSTRM(ctx, embyDir, publicURL, strmToken)
}
