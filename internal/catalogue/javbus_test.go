package catalogue

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/javbus"
	"github.com/ppxb/miyabi/internal/magnet"
)

type detailCountingProvider struct {
	stubProviderWithMagnets
	detailCalls atomic.Int32
}

func (d *detailCountingProvider) MovieDetail(ctx context.Context, id string) (domain.MovieDetail, error) {
	d.detailCalls.Add(1)
	return d.stubProviderWithMagnets.MovieDetail(ctx, id)
}

func TestMagnets_WithAvailableJavBus(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	provider := &detailCountingProvider{
		stubProviderWithMagnets: stubProviderWithMagnets{
			magnets: []domain.Magnet{{Hash: "1111111111111111111111111111111111111111", Name: "SSIS-001"}},
		},
	}
	service, err := NewWithProvider(t.Context(), store.Client, provider, &stubLocalState{})
	if err != nil {
		t.Fatal(err)
	}

	javbusClient := javbus.NewForTest(true)
	defer javbusClient.Close()
	service.javbus = javbusClient
	service.aggregator = magnet.NewAggregator([]magnet.Source{
		provider,
		javbusClient,
	}, aggregatorTimeout, nil)

	magnets, err := service.Magnets(t.Context(), "movie-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(magnets) != 1 {
		t.Fatalf("expected 1 magnet, got %d", len(magnets))
	}
	if provider.detailCalls.Load() != 1 {
		t.Fatalf("expected 1 detail call when JavBus is available, got %d", provider.detailCalls.Load())
	}
}

func TestMagnets_WithUnavailableJavBus(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	provider := &detailCountingProvider{
		stubProviderWithMagnets: stubProviderWithMagnets{
			magnets: []domain.Magnet{{Hash: "1111111111111111111111111111111111111111", Name: "SSIS-001"}},
		},
	}
	service, err := NewWithProvider(t.Context(), store.Client, provider, &stubLocalState{})
	if err != nil {
		t.Fatal(err)
	}

	javbusClient := javbus.NewForTest(false)
	defer javbusClient.Close()
	service.javbus = javbusClient
	service.aggregator = magnet.NewAggregator([]magnet.Source{
		provider,
		javbusClient,
	}, aggregatorTimeout, nil)

	magnets, err := service.Magnets(t.Context(), "movie-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(magnets) != 1 {
		t.Fatalf("expected 1 magnet from JavDB, got %d", len(magnets))
	}
	// Item 143: CatalogueDetail must NOT be queried when JavBus is not available.
	if provider.detailCalls.Load() != 0 {
		t.Fatalf("expected 0 detail calls when JavBus is unavailable, got %d", provider.detailCalls.Load())
	}
}
