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

func (*recoveringJavBusSource) Name() string    { return domain.MagnetSourceJavBus }
func (*recoveringJavBusSource) Available() bool { return true }
func (*recoveringJavBusSource) Close()          {}
func (s *recoveringJavBusSource) Find(_ context.Context, ref domain.MovieRef) ([]domain.Magnet, error) {
	s.calls++
	if s.calls == 1 {
		return nil, errors.New("temporary JavBus failure")
	}
	if ref.Code != "SSIS-001" || ref.JavDBID != "movie-1" {
		return nil, errors.New("missing primary catalogue identity")
	}
	return []domain.Magnet{{Hash: "2222222222222222222222222222222222222222", Name: "supplement"}}, nil
}

func TestMagnetsOptionalSourceFailureDoesNotCachePartialResults(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	primary := &stubProviderWithMagnets{magnets: []domain.Magnet{{Hash: "1111111111111111111111111111111111111111", Name: "primary"}}}
	supplement := &recoveringJavBusSource{}
	service, err := NewWithClients(t.Context(), store.Client, primary, supplement, &stubLocalState{})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	for _, want := range []int{1, 2, 2} {
		magnets, err := service.Magnets(t.Context(), "movie-1")
		if err != nil || len(magnets) != want {
			t.Fatalf("magnets=%+v error=%v want=%d", magnets, err, want)
		}
	}
	if supplement.calls != 2 {
		t.Fatalf("expected retry then cache hit, got %d upstream calls", supplement.calls)
	}
}

type countingJavBusHTTP struct{ calls atomic.Int32 }

type recoveringDetailProvider struct {
	stubProviderWithMagnets
	calls int
}

func (p *recoveringDetailProvider) MovieDetail(ctx context.Context, id string) (domain.MovieDetail, error) {
	p.calls++
	if p.calls == 1 {
		return domain.MovieDetail{}, errors.New("temporary detail failure")
	}
	return p.stubProviderWithMagnets.MovieDetail(ctx, id)
}

func TestMagnetsRetriesDetailBeforeCachingSupplementedResult(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	primary := &recoveringDetailProvider{stubProviderWithMagnets: stubProviderWithMagnets{
		magnets: []domain.Magnet{{Hash: "1111111111111111111111111111111111111111", Name: "primary"}},
	}}
	upstream := &countingJavBusHTTP{}
	service, err := NewWithClients(t.Context(), store.Client, primary, javbus.NewForTest(true, upstream), &stubLocalState{})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	for i, wantDetailCalls := range []int{1, 2, 2} {
		magnets, err := service.Magnets(t.Context(), "movie-1")
		if err != nil || len(magnets) != 1 {
			t.Fatalf("request %d: magnets=%+v error=%v", i, magnets, err)
		}
		if primary.calls != wantDetailCalls {
			t.Fatalf("request %d: detail calls=%d want=%d", i, primary.calls, wantDetailCalls)
		}
		wantUpstreamCalls := int32(1)
		if i == 0 {
			wantUpstreamCalls = 0
		}
		if got := upstream.calls.Load(); got != wantUpstreamCalls {
			t.Fatalf("request %d: supplement calls=%d want=%d", i, got, wantUpstreamCalls)
		}
	}
}

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
			magnets: []domain.Magnet{{Hash: "1111111111111111111111111111111111111111", Name: "SSIS-001"}},
		},
	}
	upstream := &countingJavBusHTTP{}
	javbusClient := javbus.NewForTest(true, upstream)
	service, err := NewWithClients(t.Context(), store.Client, provider, javbusClient, &stubLocalState{})
	if err != nil {
		javbusClient.Close()
		t.Fatal(err)
	}
	defer service.Close()

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
	// Repeated page loads must not reach JavBus while the outer result is cached.
	for range 3 {
		cached, err := service.Magnets(t.Context(), "movie-1")
		if err != nil || len(cached) != 1 {
			t.Fatalf("cached magnets: %+v %v", cached, err)
		}
	}
	if calls := upstream.calls.Load(); calls != 1 {
		t.Fatalf("cached requests reached JavBus %d times", calls)
	}
	service.magnets.reset()
	if _, err := service.Magnets(t.Context(), "movie-1"); err != nil {
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
			magnets: []domain.Magnet{{Hash: "1111111111111111111111111111111111111111", Name: "SSIS-001"}},
		},
	}
	javbusClient := javbus.NewForTest(false)
	service, err := NewWithClients(t.Context(), store.Client, provider, javbusClient, &stubLocalState{})
	if err != nil {
		javbusClient.Close()
		t.Fatal(err)
	}
	defer service.Close()

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
