package export

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
)

// Config holds the unified configuration for exporting media and STRM files to Emby.
type Config struct {
	EmbyDir   string
	PublicURL string
	STRMToken string
}

// Validate requires resolved destinations before export files can be written.
// Defaults are supplied by the application configuration, not by exporters.
func (c Config) Validate() error {
	if strings.TrimSpace(c.EmbyDir) == "" {
		return errors.New("export directory must not be empty")
	}
	if strings.TrimRight(strings.TrimSpace(c.PublicURL), "/") == "" {
		return errors.New("export public URL must not be empty")
	}
	return nil
}

// Manager coordinates atomic access to the export configuration and serializes STRM rewrites.
type Manager struct {
	cfg     atomic.Pointer[Config]
	writeMu sync.Mutex
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
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	m.cfg.Store(&cfg)
}

// WithConfig serializes filesystem work with configuration changes and rewrites.
// The callback must not call Set or WithConfig on this manager.
func (m *Manager) WithConfig(fn func(Config) error) error {
	if m == nil {
		return fn(Config{})
	}
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	return fn(m.Config())
}

// RewriteSTRM rewrites with the latest configuration while excluding exports.
func (m *Manager) RewriteSTRM(ctx context.Context) (int, error) {
	var count int
	err := m.WithConfig(func(cfg Config) error {
		var err error
		count, err = RewriteSTRM(ctx, cfg.EmbyDir, cfg.PublicURL, cfg.STRMToken)
		return err
	})
	return count, err
}
