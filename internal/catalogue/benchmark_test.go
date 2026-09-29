package catalogue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

var benchmarkCatalogueResult any

func BenchmarkResponseCache(b *testing.B) {
	cache := newResponseCache[string](128, time.Minute)
	ctx := b.Context()
	load := func(ctx context.Context) (string, error) {
		return "cached-value", nil
	}
	b.ReportAllocs()
	for b.Loop() {
		val, err := cache.get(ctx, "benchmark-key", load)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkCatalogueResult = val
	}
}

func BenchmarkMovieProjection(b *testing.B) {
	movies := make([]domain.Movie, 50)
	for i := range 50 {
		movies[i] = domain.Movie{
			ID:          fmt.Sprintf("movie-%d", i),
			Code:        fmt.Sprintf("ABP-%03d", i),
			Title:       "Benchmark movie title",
			ReleaseDate: "2026-01-01",
		}
	}
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		projected := projectMovies(ctx, movies)
		benchmarkCatalogueResult = projected
	}
}
