package network

import (
	"context"
	"fmt"
	"sync"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/netx"
)

const networkProxySetting = "network.proxy"

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
