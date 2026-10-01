package scan

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
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
	matcher, err := loadMovieMatcher(ctx, tx, codes)
	if err != nil {
		return nil, err
	}
	matched := make(map[string]int, len(codes))
	for _, code := range codes {
		if record := matcher.find(code); record != nil {
			matched[code] = record.ID
		}
	}
	return matched, nil
}

// A transaction-local index also lets local imports see identity changes from
// earlier NFOs in the batch without querying the same candidate groups again.
type movieMatcher map[string][]*ent.Movie

func loadMovieMatcher(ctx context.Context, tx *ent.Tx, codes []string) (movieMatcher, error) {
	var keys []string
	groups := make(movieMatcher, len(codes))
	for _, code := range codes {
		for _, key := range codeid.Queries(codeid.MatchKey(code)) {
			if _, found := groups[key]; !found {
				keys = append(keys, key)
				groups[key] = nil
			}
		}
	}
	records, err := tx.Movie.Query().Where(movie.CanonicalCodeIn(keys...)).
		Select(movie.FieldID, movie.FieldCode, movie.FieldCanonicalCode, movie.FieldJavdbID).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load equivalent movies: %w", err)
	}

	for _, record := range records {
		groups.add(record)
	}
	return groups, nil
}

func (matcher movieMatcher) find(code string) *ent.Movie {
	var best *ent.Movie
	for _, key := range codeid.Queries(codeid.MatchKey(code)) {
		for _, record := range matcher[key] {
			if !codeid.IsEquivalent(record.Code, code) {
				continue
			}
			if best == nil || movieRank(record, code) > movieRank(best, code) ||
				(movieRank(record, code) == movieRank(best, code) && record.ID < best.ID) {
				best = record
			}
		}
	}
	return best
}

func (matcher movieMatcher) add(record *ent.Movie) {
	matcher[record.CanonicalCode] = append(matcher[record.CanonicalCode], record)
}

func (matcher movieMatcher) remove(record *ent.Movie) {
	matcher[record.CanonicalCode] = slices.DeleteFunc(matcher[record.CanonicalCode], func(candidate *ent.Movie) bool {
		return candidate.ID == record.ID
	})
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
	seen := make(map[string]bool, len(codes))
	for _, code := range codes {
		if _, found := matched[code]; !found && !seen[code] {
			seen[code] = true
			pending = append(pending, code)
		}
	}
	slices.SortFunc(pending, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(a), len(b)), cmp.Compare(a, b))
	})
	owners := make(map[string][]string)
	aliases := make(map[string]string)
	builders := make([]*ent.MovieCreate, 0, len(pending))
	for _, code := range pending {
		key := codeid.MatchKey(code)
		var group []string
		for _, variant := range codeid.Queries(key) {
			group = append(group, owners[variant]...)
		}
		if index := slices.IndexFunc(group, func(owner string) bool { return codeid.IsEquivalent(owner, code) }); index >= 0 {
			aliases[code] = group[index]
			continue
		}
		owners[key] = append(owners[key], code)
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
