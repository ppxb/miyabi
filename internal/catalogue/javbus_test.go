package catalogue

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	http "github.com/bogdanfinn/fhttp"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/javbus"
)

type detailCountingProvider struct {
	stubProviderWithMagnets
	detailCalls atomic.Int32
}

type recoveringJavBusSource struct{ calls int }

func (*recoveringJavBusSource) Name() string { return domain.MagnetSourceJavBus }
func (*recoveringJavBusSource) Close()       {}
func (s *recoveringJavBusSource) Find(_ context.Context, ref domain.MovieRef) ([]domain.Magnet, error) {
	s.calls++
	if s.calls == 1 {
		return nil, errors.New("temporary JavBus failure")
	}
	if ref.Code != "SSIS-001" {
		return nil, errors.New("missing primary catalogue identity")
	}
	return []domain.Magnet{{Hash: "2222222222222222222222222222222222222222", Name: "supplement", Sources: []string{domain.MagnetSourceJavBus}}}, nil
}

func TestMagnetsOptionalSourceFailureIsCachedUntilReset(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	primary := &stubProviderWithMagnets{magnets: []domain.Magnet{{Hash: "1111111111111111111111111111111111111111", Name: "primary", Sources: []string{"javdb"}}}}
	supplement := &recoveringJavBusSource{}
	service, err := NewWithClients(t.Context(), store.Client, primary, supplement, &stubLocalState{}, primary)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	partial, err := service.CatalogueMagnetsByCode(t.Context(), "SSIS-001")
	if err != nil || len(partial) != 1 || partial[0].Hash != primary.magnets[0].Hash {
		t.Fatalf("primary magnet unavailable during partial failure: magnets=%+v error=%v", partial, err)
	}
	// A partial result is cached now: the next lookup must not re-query the
	// source that failed.
	cached, err := service.CatalogueMagnetsByCode(t.Context(), "SSIS-001")
	if err != nil || len(cached) != 1 {
		t.Fatalf("partial result was not cached: magnets=%+v error=%v", cached, err)
	}
	if supplement.calls != 1 {
		t.Fatalf("cached partial lookup reached JavBus %d times", supplement.calls)
	}
	// Only an invalidation makes the recovered source show up.
	service.magnets.reset()
	domainMagnets, err := service.CatalogueMagnetsByCode(t.Context(), "SSIS-001")
	if err != nil || len(domainMagnets) != 2 {
		t.Fatalf("reset did not pick up recovered source: magnets=%+v error=%v", domainMagnets, err)
	}
	if supplement.calls != 2 {
		t.Fatalf("expected one retry after reset, got %d upstream calls", supplement.calls)
	}
}

type countingJavBusHTTP struct{ calls atomic.Int32 }

func (c *countingJavBusHTTP) Do(*http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
}

func (*countingJavBusHTTP) CloseIdleConnections() {}

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
			magnets: []domain.Magnet{{Hash: "1111111111111111111111111111111111111111", Name: "SSIS-001", Sources: []string{"javdb"}}},
		},
	}
	upstream := &countingJavBusHTTP{}
	javbusClient := javbus.NewForTest(upstream)
	service, err := NewWithClients(t.Context(), store.Client, provider, javbusClient, &stubLocalState{}, provider)
	if err != nil {
		javbusClient.Close()
		t.Fatal(err)
	}
	defer service.Close()

	magnets, err := service.MagnetsByCode(t.Context(), "SSIS-001", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(magnets) != 1 {
		t.Fatalf("expected 1 magnet, got %d", len(magnets))
	}
	if provider.detailCalls.Load() != 0 {
		t.Fatalf("code lookups must not fetch JavDB detail, got %d calls", provider.detailCalls.Load())
	}
	// Repeated page loads must not reach JavBus while the outer result is cached.
	for range 3 {
		cached, err := service.MagnetsByCode(t.Context(), "SSIS-001", "")
		if err != nil || len(cached) != 1 {
			t.Fatalf("cached magnets: %+v %v", cached, err)
		}
	}
	if calls := upstream.calls.Load(); calls != 1 {
		t.Fatalf("cached requests reached JavBus %d times", calls)
	}
	service.magnets.reset()
	if _, err := service.MagnetsByCode(t.Context(), "SSIS-001", ""); err != nil {
		t.Fatal(err)
	}
	if calls := upstream.calls.Load(); calls != 2 {
		t.Fatalf("invalidated cache did not refresh JavBus: %d calls", calls)
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
			magnets: []domain.Magnet{{Hash: "1111111111111111111111111111111111111111", Name: "SSIS-001", Sources: []string{"javdb"}}},
		},
	}
	javbusClient := javbus.NewForTest()
	service, err := NewWithClients(t.Context(), store.Client, provider, javbusClient, &stubLocalState{}, provider)
	if err != nil {
		javbusClient.Close()
		t.Fatal(err)
	}
	defer service.Close()

	magnets, err := service.MagnetsByCode(t.Context(), "SSIS-001", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(magnets) != 1 {
		t.Fatalf("expected 1 magnet from JavDB, got %d", len(magnets))
	}
	if provider.detailCalls.Load() != 0 {
		t.Fatalf("code lookups must not fetch JavDB detail, got %d calls", provider.detailCalls.Load())
	}
}
