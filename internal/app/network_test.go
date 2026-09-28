package app

import (
	"fmt"
	"sync"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/netx"
)

func TestNetworkServiceDefaultsToDirectAndPersistsUpdates(t *testing.T) {
	directory := t.TempDir()
	store, err := database.Open(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service, err := NewNetworkService(t.Context(), store.Client)
	if err != nil {
		t.Fatal(err)
	}
	config, err := service.Network(t.Context())
	if err != nil || config != (netx.ProxyConfig{}) || service.ProxyManager().Resolve() != nil {
		t.Fatalf("initial network config = %+v, err=%v", config, err)
	}

	want := netx.ProxyConfig{Enabled: true, URL: "http://127.0.0.1:7890"}
	if err := service.UpdateNetwork(t.Context(), want); err != nil {
		t.Fatal(err)
	}
	if got, _ := service.Network(t.Context()); got != want {
		t.Fatalf("updated config = %+v, want %+v", got, want)
	}

	restarted, err := NewNetworkService(t.Context(), store.Client)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := restarted.Network(t.Context()); got != want || restarted.ProxyManager().Resolve() == nil {
		t.Fatalf("persisted config = %+v", got)
	}
}

func TestNetworkServiceRejectsInvalidUpdateWithoutChangingCurrentValue(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service, err := NewNetworkService(t.Context(), store.Client)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateNetwork(t.Context(), netx.ProxyConfig{Enabled: true, URL: "ftp://127.0.0.1:21"}); err == nil {
		t.Fatal("invalid proxy URL was accepted")
	}
	if got, _ := service.Network(t.Context()); got != (netx.ProxyConfig{}) {
		t.Fatalf("invalid update changed config: %+v", got)
	}
}

func TestNetworkServiceConcurrentUpdatesStayConsistent(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service, err := NewNetworkService(t.Context(), store.Client)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 10
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := range workers {
		go func(n int) {
			defer wg.Done()
			config := netx.ProxyConfig{Enabled: n%2 == 0, URL: fmt.Sprintf("http://127.0.0.1:%d", 7000+n)}
			_ = service.UpdateNetwork(t.Context(), config)
		}(i)
	}
	wg.Wait()

	// Memory config must match the persisted database config
	memConfig, err := service.Network(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	dbConfig, _, err := database.LoadSetting[netx.ProxyConfig](t.Context(), store.Client, networkProxySetting)
	if err != nil {
		t.Fatal(err)
	}
	if memConfig != dbConfig {
		t.Fatalf("memory config %+v does not match DB config %+v", memConfig, dbConfig)
	}
}
