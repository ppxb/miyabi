package library

import (
	"fmt"
	"testing"
	"time"
)

// BenchmarkAddViewedMovieIDs includes input generation and the complete write
// transaction on a temporary SQLite WAL database, excluding schema and seed setup.
func BenchmarkAddViewedMovieIDs(b *testing.B) {
	for _, tc := range []struct {
		name  string
		count int
		batch int
	}{
		{"small-refresh", 100, 0},
		{"full-refresh", maxViewedMovies, 0},
		{"full-new-one", maxViewedMovies, 1},
		{"full-new-batch", maxViewedMovies, 1000},
	} {
		b.Run(tc.name, func(b *testing.B) {
			svc := viewedFixture(b, tc.count, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
			iteration := 0
			b.ReportAllocs()
			for b.Loop() {
				ids := []string{"seed-0"}
				if tc.batch > 0 {
					ids = make([]string, tc.batch)
					for i := range ids {
						ids[i] = fmt.Sprintf("new-%d-%d", iteration, i)
					}
				}
				if err := svc.AddViewedMovieIDs(b.Context(), ids); err != nil {
					b.Fatal(err)
				}
				iteration++
			}
			if count := svc.database.ViewedMovie.Query().CountX(b.Context()); count != tc.count {
				b.Fatalf("history size=%d, want %d", count, tc.count)
			}
		})
	}
}
