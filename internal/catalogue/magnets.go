package catalogue

import (
	"context"
	"fmt"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
)

// Magnets returns the aggregated magnet list with URIs for the API.
func (service *Service) Magnets(ctx context.Context, movieID string) ([]Magnet, error) {
	magnets, err := service.cachedMagnets(ctx, movieID)
	if err != nil {
		return nil, err
	}
	return projectMagnets(magnets), nil
}

// CatalogueMagnets returns domain magnets in a separate slice for internal callers.
func (service *Service) CatalogueMagnets(ctx context.Context, movieID string) ([]domain.Magnet, error) {
	magnets, err := service.cachedMagnets(ctx, movieID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Magnet, len(magnets))
	copy(result, magnets)
	return result, nil
}

// CatalogueMagnetsByCode returns domain magnets for a normalized catalogue code
// without resolving a JavDB movie ID through movie detail. Subscription and
// offline recovery use it so magnet lookup no longer depends on JavDB detail.
func (service *Service) CatalogueMagnetsByCode(ctx context.Context, code string) ([]domain.Magnet, error) {
	code = codeid.Normalize(code)
	if code == "" {
		return nil, nil
	}
	key := "code:" + code
	magnets, err := cachedJavDB(ctx, service, service.magnets, key, func(ctx context.Context) ([]domain.Magnet, error) {
		magnets, partial, err := service.aggregator.FindDetailed(ctx, domain.MovieRef{Code: code})
		if err != nil {
			return nil, err
		}
		if partial {
			return magnets, ErrDoNotCache
		}
		return magnets, nil
	})
	if err != nil {
		return nil, fmt.Errorf("get magnets by code: %w", err)
	}
	return magnets, nil
}

// MagnetsByCode returns the API magnet list for a catalogue code, bypassing the
// JavDB detail lookup used by the movie-ID path.
func (service *Service) MagnetsByCode(ctx context.Context, code string) ([]Magnet, error) {
	magnets, err := service.CatalogueMagnetsByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	return projectMagnets(magnets), nil
}

func (service *Service) HasMagnet(ctx context.Context, movieID, hash string) (bool, error) {
	magnets, err := service.cachedMagnets(ctx, movieID)
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

// cachedMagnets shares immutable domain data across all magnet lookups. The detail
// lookup supplies the code that the code-based sources need; when it fails they
// cannot contribute and the result is returned without caching so it retries.
func (service *Service) cachedMagnets(ctx context.Context, movieID string) ([]domain.Magnet, error) {
	magnets, err := cachedJavDB(ctx, service, service.magnets, movieID, func(ctx context.Context) ([]domain.Magnet, error) {
		ref := domain.MovieRef{}
		detailFailed := false
		if service.sukebei != nil || service.javbus != nil {
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
	return magnets, nil
}

func projectMagnets(source []domain.Magnet) []Magnet {
	result := make([]Magnet, len(source))
	for index, item := range source {
		result[index] = Magnet{Magnet: item, URI: "magnet:?xt=urn:btih:" + item.Hash}
	}
	return result
}
