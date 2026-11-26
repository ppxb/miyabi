package catalogue

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/movie"
)

// CatalogueMagnetsByCode returns domain magnets for a catalogue code in a
// separate slice for internal callers. JavDB joins only when the library has
// recorded the movie's JavDB id.
func (service *Service) CatalogueMagnetsByCode(ctx context.Context, code string) ([]domain.Magnet, error) {
	magnets, err := service.magnetsByCode(ctx, code, "")
	return slices.Clone(magnets), err
}

// MagnetsByCode returns the API magnet list for a catalogue code. javdbID comes
// from a JavDB catalogue card; without it the library's recorded id is used,
// and JavDB is skipped when neither exists.
func (service *Service) MagnetsByCode(ctx context.Context, code, javdbID string) ([]Magnet, error) {
	magnets, err := service.magnetsByCode(ctx, code, javdbID)
	if err != nil {
		return nil, err
	}
	return projectMagnets(magnets), nil
}

func (service *Service) magnetsByCode(ctx context.Context, code, javdbID string) ([]domain.Magnet, error) {
	code = codeid.Normalize(code)
	if code == "" {
		return nil, nil
	}
	javdbID = strings.TrimSpace(javdbID)
	if javdbID == "" {
		if record, err := service.database.Movie.Query().Where(movie.CodeEQ(code)).Only(ctx); err == nil && record.Metadata != nil {
			javdbID = record.Metadata.SourceID(domain.MagnetSourceJavDB)
		}
	}
	key := "code:" + code
	if javdbID != "" {
		// A result without JavDB must not answer a request that can include it.
		key += "|javdb:" + javdbID
	}
	ref := domain.MovieRef{Code: code, JavdbID: javdbID}
	// Partial results are cached too: a missing source is usually a lasting
	// block or cooldown, and re-querying every source on each page load is what
	// made this cache ineffective. Empty results expire sooner so a release
	// whose magnets have not appeared yet is picked up quickly.
	magnets, err := cachedJavDB(ctx, service, service.magnets, key, func(ctx context.Context) ([]domain.Magnet, time.Duration, error) {
		magnets, _, err := service.aggregator.FindDetailed(ctx, ref)
		if err != nil {
			return nil, 0, err
		}
		if len(magnets) == 0 {
			return magnets, magnetsEmptyTTL, nil
		}
		return magnets, 0, nil
	})
	if err != nil {
		return nil, fmt.Errorf("get magnets by code: %w", err)
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
