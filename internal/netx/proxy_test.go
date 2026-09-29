package netx

import (
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

func TestNewProxyManagerDefaultsToDirect(t *testing.T) {
	manager, err := NewProxyManager(ProxyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if got := manager.Resolve(); got != nil {
		t.Fatalf("Resolve() = %v, want nil", got)
	}
	if got := manager.Config(); got != (ProxyConfig{}) {
		t.Fatalf("Config() = %+v, want zero config", got)
	}
}

func TestProxyManagerValidatesAndNormalizesURL(t *testing.T) {
	manager, err := NewProxyManager(ProxyConfig{Enabled: true, URL: " HTTP://user:pass@127.0.0.1:7890 "})
	if err != nil {
		t.Fatal(err)
	}
	proxy := manager.Resolve()
	if proxy == nil || proxy.Scheme != "http" || proxy.Host != "127.0.0.1:7890" || proxy.User.String() != "user:pass" {
		t.Fatalf("Resolve() = %v", proxy)
	}
	if got := manager.Config(); got.URL != "HTTP://user:pass@127.0.0.1:7890" || !got.Enabled {
		t.Fatalf("Config() = %+v", got)
	}
}

func TestProxyManagerAllowsDisabledURL(t *testing.T) {
	manager, err := NewProxyManager(ProxyConfig{URL: "socks5://127.0.0.1:1080"})
	if err != nil {
		t.Fatal(err)
	}
	if got := manager.Resolve(); got != nil {
		t.Fatalf("Resolve() = %v, want nil while disabled", got)
	}
	if err := manager.Update(ProxyConfig{Enabled: true, URL: "socks5://127.0.0.1:1080"}); err != nil {
		t.Fatal(err)
	}
	if got := manager.Resolve(); got == nil || got.Scheme != "socks5" {
		t.Fatalf("Resolve() after enabling = %v", got)
	}
}

func TestProxyManagerRejectsInvalidConfigurations(t *testing.T) {
	for _, test := range []struct {
		name   string
		config ProxyConfig
	}{
		{name: "missing scheme", config: ProxyConfig{URL: "127.0.0.1:7890"}},
		{name: "missing host", config: ProxyConfig{URL: "http://"}},
		{name: "unsupported scheme", config: ProxyConfig{URL: "ftp://127.0.0.1:21"}},
		{name: "malformed URL", config: ProxyConfig{URL: "http://[::1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewProxyManager(test.config); !domain.IsKind(err, domain.KindInvalid) {
				t.Fatalf("NewProxyManager() error = %v, want domain.KindInvalid", err)
			}
		})
	}
}

func TestProxyManagerEnabledWithoutURLDefaultsToDirect(t *testing.T) {
	manager, err := NewProxyManager(ProxyConfig{Enabled: true, URL: ""})
	if err != nil {
		t.Fatal(err)
	}
	if got := manager.Resolve(); got != nil {
		t.Fatalf("Resolve() = %v, want nil for empty URL", got)
	}
}

func TestProxyManagerUpdateNotifiesSubscribers(t *testing.T) {
	manager, err := NewProxyManager(ProxyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	subscriber := manager.Subscribe()
	if err := manager.Update(ProxyConfig{Enabled: true, URL: "https://127.0.0.1:7890"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-subscriber:
	case <-time.After(time.Second):
		t.Fatal("subscriber was not notified")
	}
	if got := manager.Resolve(); got == nil || got.Scheme != "https" {
		t.Fatalf("Resolve() = %v after update", got)
	}
	select {
	case <-subscriber:
		t.Fatal("notification was not coalesced")
	default:
	}
	if err := manager.Update(ProxyConfig{Enabled: false, URL: "https://127.0.0.1:7890"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-subscriber:
	case <-time.After(time.Second):
		t.Fatal("subscriber was not notified after disabling")
	}
}

func TestProxyManagerResolveReturnsCopy(t *testing.T) {
	manager, err := NewProxyManager(ProxyConfig{Enabled: true, URL: "http://127.0.0.1:7890"})
	if err != nil {
		t.Fatal(err)
	}
	first := manager.Resolve()
	first.Host = "127.0.0.1:1"
	second := manager.Resolve()
	if second == nil || second.Host != "127.0.0.1:7890" {
		t.Fatalf("Resolve() was mutated through returned URL: %v", second)
	}
}

func TestProxyManagerNotifiesOnlyForEffectiveChanges(t *testing.T) {
	manager, err := NewProxyManager(ProxyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	updates := manager.Subscribe()
	defer manager.Unsubscribe(updates)
	for _, step := range []struct {
		name   string
		config ProxyConfig
		active string
		notify bool
	}{
		{"save disabled URL", ProxyConfig{URL: "http://127.0.0.1:7890"}, "", false},
		{"edit disabled URL", ProxyConfig{URL: "http://127.0.0.1:7891"}, "", false},
		{"enable", ProxyConfig{Enabled: true, URL: "http://127.0.0.1:7891"}, "http://127.0.0.1:7891", true},
		{"same proxy", ProxyConfig{Enabled: true, URL: "http://127.0.0.1:7891"}, "http://127.0.0.1:7891", false},
		{"equivalent scheme", ProxyConfig{Enabled: true, URL: "HTTP://127.0.0.1:7891"}, "http://127.0.0.1:7891", false},
		{"switch proxy", ProxyConfig{Enabled: true, URL: "http://127.0.0.1:7892"}, "http://127.0.0.1:7892", true},
		{"disable", ProxyConfig{URL: "http://127.0.0.1:7892"}, "", true},
		{"clear disabled URL", ProxyConfig{}, "", false},
	} {
		t.Run(step.name, func(t *testing.T) {
			if err := manager.Update(step.config); err != nil {
				t.Fatal(err)
			}
			if got := manager.Config(); got != step.config {
				t.Fatalf("saved config = %+v, want %+v", got, step.config)
			}
			var active string
			if proxy := manager.Resolve(); proxy != nil {
				active = proxy.String()
			}
			if active != step.active {
				t.Fatalf("active proxy = %q, want %q", active, step.active)
			}
			var notified bool
			select {
			case <-updates:
				notified = true
			default:
			}
			if notified != step.notify {
				t.Fatalf("notification = %t, want %t", notified, step.notify)
			}
		})
	}
}

func TestNormalizeTrimsAndResolves(t *testing.T) {
	config, proxy, err := Normalize(ProxyConfig{Enabled: true, URL: " http://127.0.0.1:7890 "})
	if err != nil || config.URL != "http://127.0.0.1:7890" || proxy == nil || proxy.Host != "127.0.0.1:7890" {
		t.Fatalf("Normalize() = %+v, %v, %v", config, proxy, err)
	}
	if _, proxy, err := Normalize(ProxyConfig{URL: "http://127.0.0.1:7890"}); err != nil || proxy != nil {
		t.Fatalf("Normalize(disabled) proxy = %v, err = %v", proxy, err)
	}
}
