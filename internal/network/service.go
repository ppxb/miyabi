package network

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

// Service owns the persisted upstream proxy configuration. The proxy
// applies to JavDB and JavBus alike; catalogue decides when to query JavBus.
type Service struct {
	database *ent.Client
	proxy    *netx.ProxyManager
	mu       sync.Mutex
}

func New(ctx context.Context, db *ent.Client) (*Service, error) {
	config, _, err := database.LoadSetting[netx.ProxyConfig](ctx, db, networkProxySetting)
	if err != nil {
		return nil, err
	}
	proxy, err := netx.NewProxyManager(config)
	if err != nil {
		return nil, fmt.Errorf("load network proxy setting: %w", err)
	}
	return &Service{database: db, proxy: proxy}, nil
}

func (service *Service) ProxyManager() *netx.ProxyManager {
	return service.proxy
}

func (service *Service) Config() netx.ProxyConfig {
	return service.proxy.Config()
}

// UpdateNetwork validates, persists and broadcasts the configuration.
func (service *Service) UpdateNetwork(ctx context.Context, config netx.ProxyConfig) error {
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
func (service *Service) TestNetwork(ctx context.Context, config netx.ProxyConfig) (TestResult, error) {
	_, proxy, err := netx.Normalize(config)
	if err != nil {
		return TestResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, networkProbeTimeout)
	defer cancel()

	var response TestResult
	var wait sync.WaitGroup
	probe := func(target *ProbeResult, measure func(context.Context, *url.URL, time.Duration) (time.Duration, error)) {
		defer wait.Done()
		latency, err := measure(ctx, proxy, networkProbeTimeout)
		if err != nil {
			*target = ProbeResult{Error: err.Error()}
			return
		}
		*target = ProbeResult{Available: true, LatencyMS: latency.Milliseconds()}
	}
	wait.Add(2)
	go probe(&response.JavDB, javdb.Probe)
	go probe(&response.JavBus, javbus.Probe)
	wait.Wait()
	return response, nil
}
