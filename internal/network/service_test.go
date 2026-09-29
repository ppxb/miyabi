package network

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

	service, err := New(t.Context(), store.Client)
	if err != nil {
		t.Fatal(err)
	}
	config := service.Config()
	if config != (netx.ProxyConfig{}) || service.ProxyManager().Resolve() != nil {
		t.Fatalf("initial network config = %+v", config)
	}

	want := netx.ProxyConfig{Enabled: true, URL: "http://127.0.0.1:7890"}
	if err := service.UpdateNetwork(t.Context(), want); err != nil {
		t.Fatal(err)
	}
	if got := service.Config(); got != want {
		t.Fatalf("updated config = %+v, want %+v", got, want)
	}

	restarted, err := New(t.Context(), store.Client)
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.Config(); got != want || restarted.ProxyManager().Resolve() == nil {
		t.Fatalf("persisted config = %+v", got)
	}
}

func TestNetworkServiceRejectsInvalidUpdateWithoutChangingCurrentValue(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service, err := New(t.Context(), store.Client)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateNetwork(t.Context(), netx.ProxyConfig{Enabled: true, URL: "ftp://127.0.0.1:21"}); err == nil {
		t.Fatal("invalid proxy URL was accepted")
	}
	if got := service.Config(); got != (netx.ProxyConfig{}) {
		t.Fatalf("invalid update changed config: %+v", got)
	}
}

func TestNetworkServiceConcurrentUpdatesStayConsistent(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service, err := New(t.Context(), store.Client)
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
	memConfig := service.Config()
	dbConfig, _, err := database.LoadSetting[netx.ProxyConfig](t.Context(), store.Client, networkProxySetting)
	if err != nil {
		t.Fatal(err)
	}
	if memConfig != dbConfig {
		t.Fatalf("memory config %+v does not match DB config %+v", memConfig, dbConfig)
	}
}
