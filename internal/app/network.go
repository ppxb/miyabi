package app

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/javbus"
	"github.com/ppxb/miyabi/internal/javdb"
	"github.com/ppxb/miyabi/internal/netx"
)

const (
	networkProxySetting = "network.proxy"
	networkProbeTimeout = 8 * time.Second
)

// NetworkService owns the persisted upstream proxy configuration. The proxy
// applies to JavDB and JavBus alike; whether JavBus is queried at all is a
// catalogue setting, not a network one.
type NetworkService struct {
	database *ent.Client
	proxy    *netx.ProxyManager
	mu       sync.Mutex
}

func NewNetworkService(ctx context.Context, db *ent.Client) (*NetworkService, error) {
	config, _, err := database.LoadSetting[netx.ProxyConfig](ctx, db, networkProxySetting)
	if err != nil {
		return nil, err
	}
	proxy, err := netx.NewProxyManager(config)
	if err != nil {
		return nil, fmt.Errorf("load network proxy setting: %w", err)
	}
	return &NetworkService{database: db, proxy: proxy}, nil
}

func (service *NetworkService) ProxyManager() *netx.ProxyManager {
	return service.proxy
}

func (service *NetworkService) Network(context.Context) (netx.ProxyConfig, error) {
	return service.proxy.Config(), nil
}

// UpdateNetwork validates, persists and broadcasts the configuration.
func (service *NetworkService) UpdateNetwork(ctx context.Context, config netx.ProxyConfig) error {
	normalized, parsed, err := netx.Normalize(config)
	if err != nil {
		return err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if err := database.SaveSetting(ctx, service.database, networkProxySetting, normalized); err != nil {
		return err
	}
	service.proxy.Apply(normalized, parsed)
	return nil
}

// TestNetwork probes JavDB and JavBus concurrently through the candidate configuration without persisting it.
func (service *NetworkService) TestNetwork(ctx context.Context, config netx.ProxyConfig) (netx.NetworkTestResponse, error) {
	_, proxy, err := netx.Normalize(config)
	if err != nil {
		return netx.NetworkTestResponse{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, networkProbeTimeout)
	defer cancel()

	var response netx.NetworkTestResponse
	var wait sync.WaitGroup
	probe := func(target *netx.NetworkProbeResult, measure func(context.Context, *url.URL, time.Duration) (time.Duration, error)) {
		defer wait.Done()
		latency, err := measure(ctx, proxy, networkProbeTimeout)
		if err != nil {
			*target = netx.NetworkProbeResult{Error: err.Error()}
			return
		}
		*target = netx.NetworkProbeResult{Available: true, LatencyMS: latency.Milliseconds()}
	}
	wait.Add(2)
	go probe(&response.JavDB, javdb.Probe)
	go probe(&response.JavBus, javbus.Probe)
	wait.Wait()
	return response, nil
}
