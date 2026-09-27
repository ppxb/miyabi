package catalogue

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/magnet"
	"github.com/ppxb/miyabi/internal/netx"
)

type countingSource struct{ calls atomic.Int32 }

func (s *countingSource) Name() string { return domain.MagnetSourceJavBus }

func (s *countingSource) Find(context.Context, domain.MovieRef) ([]domain.Magnet, error) {
	s.calls.Add(1)
	return []domain.Magnet{{Hash: "2222222222222222222222222222222222222222"}}, nil
}

func TestGatedSourceFollowsSwitch(t *testing.T) {
	inner := &countingSource{}
	var enabled atomic.Bool
	source := gatedSource{Source: inner, enabled: &enabled}

	magnets, err := source.Find(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if !errors.Is(err, magnet.ErrSkipped) || len(magnets) != 0 || inner.calls.Load() != 0 {
		t.Fatalf("disabled source must return ErrSkipped: %v %v calls=%d", magnets, err, inner.calls.Load())
	}
	enabled.Store(true)
	magnets, err = source.Find(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if err != nil || len(magnets) != 1 || inner.calls.Load() != 1 {
		t.Fatalf("enabled source must be queried once: %v %v calls=%d", magnets, err, inner.calls.Load())
	}
	if source.Name() != domain.MagnetSourceJavBus {
		t.Fatalf("gated source must keep the inner name, got %s", source.Name())
	}
}

func TestUpdateJavBusPersistsAndClearsMagnetCache(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	provider := &stubProviderWithMagnets{magnets: []domain.Magnet{{Hash: "1111111111111111111111111111111111111111", Name: "SSIS-001"}}}
	service, err := NewWithProvider(t.Context(), store.Client, provider, &stubLocalState{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Magnets(t.Context(), "movie-1"); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateJavBus(t.Context(), JavBusConfig{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	config, _ := service.JavBus(t.Context())
	if !config.Enabled {
		t.Fatal("switch must flip in memory")
	}
	saved, found, err := database.LoadSetting[bool](t.Context(), store.Client, javbusEnabledSetting)
	if err != nil || !found || !saved {
		t.Fatalf("switch must persist: saved=%v found=%v err=%v", saved, found, err)
	}
	if len(service.magnets.entries) != 0 {
		t.Fatal("toggling JavBus must drop cached magnet lists")
	}
}

func TestUpdateJavBusRequiresProxy(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	proxyManager, err := netx.NewProxyManager(netx.ProxyConfig{Enabled: false, URL: ""})
	if err != nil {
		t.Fatal(err)
	}

	provider := &stubProviderWithMagnets{magnets: nil}
	service, err := NewWithProvider(t.Context(), store.Client, provider, &stubLocalState{})
	if err != nil {
		t.Fatal(err)
	}
	service.proxy = proxyManager

	// Enabling JavBus without active proxy must fail.
	err = service.UpdateJavBus(t.Context(), JavBusConfig{Enabled: true})
	if err == nil {
		t.Fatal("expected error when enabling JavBus without proxy, got nil")
	}

	// Disabling JavBus is always permitted.
	if err := service.UpdateJavBus(t.Context(), JavBusConfig{Enabled: false}); err != nil {
		t.Fatalf("expected nil when disabling JavBus, got %v", err)
	}

	// With proxy enabled, enabling JavBus must succeed.
	if err := proxyManager.Update(netx.ProxyConfig{Enabled: true, URL: "http://127.0.0.1:7890"}); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateJavBus(t.Context(), JavBusConfig{Enabled: true}); err != nil {
		t.Fatalf("expected success with proxy enabled, got %v", err)
	}
}
