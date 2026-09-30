package scan

import (
	"fmt"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent"
)

// A scan page mixes complete numbers and undecorated aliases in a larger library.
func BenchmarkMatchMovies(b *testing.B) {
	store, err := database.Open(b.Context(), b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	tx, err := store.Client.Tx(b.Context())
	if err != nil {
		b.Fatal(err)
	}
	defer tx.Rollback()
	var codes []string
	for start := 0; start < 5000; start += 100 {
		var builders []*ent.MovieCreate
		for i := start; i < start+100; i++ {
			code := fmt.Sprintf("TEST-%05d", i)
			if i%50 == 0 {
				codes = append(codes, code)
			}
			if i%100 == 0 {
				code = "200" + code
			}
			builders = append(builders, tx.Movie.Create().SetCode(code))
		}
		if _, err := tx.Movie.CreateBulk(builders...).Save(b.Context()); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		matched, err := MatchMovies(b.Context(), tx, codes)
		if err != nil || len(matched) != len(codes) {
			b.Fatalf("matched=%d want=%d err=%v", len(matched), len(codes), err)
		}
	}
}
