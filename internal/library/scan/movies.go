package scan

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/predicate"
)

// MatchMovies maps catalogue numbers to the existing movies they may name.
// Filenames can carry distributor or studio decorations that scraped records drop
// (259LUXU-1899 vs LUXU-1899), and an unscraped record can keep them while a later
// file does not, so matching follows codeid.IsEquivalent in both directions.
// A scraped record owns its catalogue number and wins over other spellings,
// then the exact number; codes without a match are absent from the result.
func MatchMovies(ctx context.Context, tx *ent.Tx, codes []string) (map[string]int, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	var candidates []string
	predicates := make([]predicate.Movie, 0, len(codes)+1)
	for _, code := range codes {
		candidates = append(candidates, codeid.Candidates(code)...)
		predicates = append(predicates, movie.CodeHasSuffix(code))
	}
	predicates = append(predicates, movie.CodeIn(candidates...))
	records, err := tx.Movie.Query().Where(movie.Or(predicates...)).Order(ent.Asc(movie.FieldID)).
		Select(movie.FieldID, movie.FieldCode, movie.FieldJavdbID).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load equivalent movies: %w", err)
	}

	matched := make(map[string]int, len(codes))
	for _, code := range codes {
		best := -1
		for _, record := range records {
			if rank := movieRank(record, code); rank > best && codeid.IsEquivalent(record.Code, code) {
				best, matched[code] = rank, record.ID
			}
		}
	}
	return matched, nil
}

// movieRank orders equivalent records: scraped first, then the exact number.
func movieRank(record *ent.Movie, code string) int {
	rank := 0
	if record.JavdbID != nil {
		rank += 2
	}
	if record.Code == code {
		rank++
	}
	return rank
}

// indexMovies binds every code to an existing or newly created movie. Equivalent
// new codes share one record under the shortest spelling, so later scrapes that
// canonicalize the number cannot collide on the unique movie code.
func indexMovies(ctx context.Context, tx *ent.Tx, codes []string) (map[string]int, error) {
	matched, err := MatchMovies(ctx, tx, codes)
	if err != nil {
		return nil, err
	}
	var pending []string
	for _, code := range codes {
		if _, found := matched[code]; !found && !slices.Contains(pending, code) {
			pending = append(pending, code)
		}
	}
	slices.SortFunc(pending, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(a), len(b)), cmp.Compare(a, b))
	})
	var owners []string
	aliases := make(map[string]string)
	builders := make([]*ent.MovieCreate, 0, len(pending))
	for _, code := range pending {
		if index := slices.IndexFunc(owners, func(owner string) bool { return codeid.IsEquivalent(owner, code) }); index >= 0 {
			aliases[code] = owners[index]
			continue
		}
		owners = append(owners, code)
		builders = append(builders, tx.Movie.Create().SetCode(code))
	}
	if len(builders) > 0 {
		created, err := tx.Movie.CreateBulk(builders...).Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("index scanned movies: %w", err)
		}
		for _, record := range created {
			matched[record.Code] = record.ID
		}
	}
	for code, owner := range aliases {
		matched[code] = matched[owner]
	}
	return matched, nil
}
