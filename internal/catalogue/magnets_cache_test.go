package catalogue

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/magnet"
)

func TestMagnetsEmptyResultsExpireAfterEmptyTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := &countingMagnetSource{name: domain.MagnetSourceProjectJav}
		broken := &failingMagnetSource{}
		service := &Service{
			javdb:      &stubProviderWithMagnets{},
			aggregator: magnet.NewAggregator([]magnet.Source{source, broken}, 0),
			magnets:    newResponseCache[[]domain.Magnet](magnetsCacheSize, magnetsCacheTTL),
		}

		for range 2 {
			magnets, err := service.magnetsByCode(t.Context(), "SSIS-001", "javdb-1")
			if err != nil || len(magnets) != 0 {
				t.Fatalf("empty lookup = %+v, err = %v", magnets, err)
			}
		}
		if calls := source.calls.Load(); calls != 1 {
			t.Fatalf("empty result was not cached, calls = %d", calls)
		}

		time.Sleep(magnetsEmptyTTL + time.Second)
		if _, err := service.magnetsByCode(t.Context(), "SSIS-001", "javdb-1"); err != nil {
			t.Fatal(err)
		}
		if calls := source.calls.Load(); calls != 2 {
			t.Fatalf("empty result outlived magnetsEmptyTTL, calls = %d", calls)
		}
	})
}

type failingMagnetSource struct{ calls atomic.Int32 }

func (*failingMagnetSource) Name() string { return domain.MagnetSourceProjectJav }

func (s *failingMagnetSource) Find(context.Context, domain.MovieRef) ([]domain.Magnet, error) {
	s.calls.Add(1)
	return nil, errors.New("source is down")
}

func TestMagnetsTotallyFailedLookupsAreNotCached(t *testing.T) {
	source := &failingMagnetSource{}
	service := &Service{
		javdb:      &stubProviderWithMagnets{},
		aggregator: magnet.NewAggregator([]magnet.Source{source}, 0),
		magnets:    newResponseCache[[]domain.Magnet](magnetsCacheSize, magnetsCacheTTL),
	}

	for range 2 {
		if _, err := service.magnetsByCode(t.Context(), "SSIS-001", "javdb-1"); err == nil {
			t.Fatal("expected an error when every source failed")
		}
	}
	if calls := source.calls.Load(); calls != 2 {
		t.Fatalf("failed lookup was cached, calls = %d", calls)
	}
	if len(service.magnets.entries) != 0 {
		t.Fatalf("failed lookup was stored, entries = %d", len(service.magnets.entries))
	}
}
