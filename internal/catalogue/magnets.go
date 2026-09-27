package catalogue

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/magnet"
)

const (
	javbusEnabledSetting = "javbus.enabled"
	aggregatorTimeout    = 8 * time.Second
)

// JavBusConfig is the JavBus settings section: one switch. JavBus has a single
// public endpoint, so there is nothing else to configure.
type JavBusConfig struct {
	Enabled bool `json:"enabled"`
}

// gatedSource hides a magnet source behind a runtime switch so the aggregator
// is assembled once and toggled without rebuilding clients.
type gatedSource struct {
	magnet.Source
	enabled *atomic.Bool
}

func (s gatedSource) Find(ctx context.Context, ref domain.MovieRef) ([]domain.Magnet, error) {
	if !s.enabled.Load() {
		return nil, magnet.ErrSkipped
	}
	return s.Source.Find(ctx, ref)
}

func (service *Service) JavBus(context.Context) (JavBusConfig, error) {
	return JavBusConfig{Enabled: service.javbusEnabled.Load()}, nil
}

// UpdateJavBus persists the switch and drops cached magnet lists so the next
// lookup reflects it.
func (service *Service) UpdateJavBus(ctx context.Context, config JavBusConfig) error {
	if config.Enabled && service.proxy != nil && service.proxy.Resolve() == nil {
		return domain.E(domain.KindInvalid, "请先开启网络代理以启用 JavBus 数据源", nil)
	}
	if err := database.SaveSetting(ctx, service.database, javbusEnabledSetting, config.Enabled); err != nil {
		return err
	}
	service.javbusEnabled.Store(config.Enabled)
	service.magnets.reset()
	return nil
}

// Magnets returns the aggregated, cached magnet list of a movie. The detail
// lookup supplies the code and zone JavBus needs; when it fails the
// aggregator still runs with the JavDB ID alone.
func (service *Service) Magnets(ctx context.Context, movieID string) ([]Magnet, error) {
	magnets, err := cachedJavDB(ctx, service, service.magnets, movieID, func(ctx context.Context) ([]domain.Magnet, error) {
		if service.aggregator == nil {
			return service.javdb.Magnets(ctx, movieID)
		}
		ref := domain.MovieRef{JavDBID: movieID}
		if detail, err := service.CatalogueDetail(ctx, movieID); err == nil {
			ref.Code, ref.Zone = detail.Code, detail.Zone
		}
		magnets, partial, err := service.aggregator.FindDetailed(ctx, ref)
		if err != nil {
			return nil, err
		}
		if partial {
			return magnets, ErrDoNotCache
		}
		return magnets, nil
	})
	if err != nil {
		return nil, fmt.Errorf("get magnets: %w", err)
	}
	return projectMagnets(magnets), nil
}

// CatalogueMagnets is Magnets without the URI projection, for packages that
// only speak domain types.
func (service *Service) CatalogueMagnets(ctx context.Context, movieID string) ([]domain.Magnet, error) {
	magnets, err := service.Magnets(ctx, movieID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Magnet, len(magnets))
	for index, item := range magnets {
		result[index] = item.Magnet
	}
	return result, nil
}

func (service *Service) HasMagnet(ctx context.Context, movieID, hash string) (bool, error) {
	magnets, err := service.Magnets(ctx, movieID)
	if err != nil {
		return false, err
	}
	for _, item := range magnets {
		if strings.EqualFold(item.Hash, hash) {
			return true, nil
		}
	}
	return false, nil
}

func projectMagnets(source []domain.Magnet) []Magnet {
	result := make([]Magnet, len(source))
	for index, item := range source {
		result[index] = Magnet{Magnet: item, URI: "magnet:?xt=urn:btih:" + item.Hash}
	}
	return result
}
