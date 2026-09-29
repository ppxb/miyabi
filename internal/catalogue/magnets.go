package catalogue

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

const aggregatorTimeout = 8 * time.Second

// Magnets returns the aggregated, cached magnet list of a movie. The detail
// lookup supplies the code and zone JavBus needs; when it fails the
// aggregator still runs with the JavDB ID alone.
func (service *Service) Magnets(ctx context.Context, movieID string) ([]Magnet, error) {
	magnets, err := cachedJavDB(ctx, service, service.magnets, movieID, func(ctx context.Context) ([]domain.Magnet, error) {
		ref := domain.MovieRef{JavDBID: movieID}
		detailFailed := false
		if service.javbus != nil && service.javbus.Available() {
			if detail, err := service.CatalogueDetail(ctx, movieID); err == nil {
				ref.Code, ref.Zone = detail.Code, detail.Zone
			} else {
				detailFailed = true
			}
		}
		magnets, partial, err := service.aggregator.FindDetailed(ctx, ref)
		if err != nil {
			return nil, err
		}
		// Missing detail prevents JavBus from participating; retry it on the next request.
		if partial || detailFailed {
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
