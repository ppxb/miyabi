package library

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/viewedmovie"
)

const maxViewedMovies = 5000

// ViewedMovieIDs returns viewed JavDB IDs, ordered by most recently viewed first.
func (s *Service) ViewedMovieIDs(ctx context.Context) ([]string, error) {
	records, err := s.database.ViewedMovie.Query().
		Order(ent.Desc(viewedmovie.FieldViewedAt), ent.Desc(viewedmovie.FieldID)).
		Limit(maxViewedMovies).
		Select(viewedmovie.FieldJavdbID).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load viewed movie IDs: %w", err)
	}
	ids := make([]string, 0, len(records))
	for _, r := range records {
		ids = append(ids, r.JavdbID)
	}
	return ids, nil
}

// AddViewedMovieIDs records viewed JavDB IDs, updating viewed_at timestamps and evicting
// the oldest entries when the table exceeds maxViewedMovies.
func (s *Service) AddViewedMovieIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().UTC()
	seen := make(map[string]struct{}, len(ids))
	var clean []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return nil
	}

	return ent.WithTx(ctx, s.database, func(tx *ent.Tx) error {
		latest, err := tx.ViewedMovie.Query().
			Order(ent.Desc(viewedmovie.FieldViewedAt)).
			First(ctx)
		if err != nil && !ent.IsNotFound(err) {
			return fmt.Errorf("load latest viewed movie: %w", err)
		}
		if latest != nil && !now.After(latest.ViewedAt) {
			now = latest.ViewedAt.Add(time.Microsecond)
		}

		var builders []*ent.ViewedMovieCreate
		for _, id := range clean {
			builders = append(builders, tx.ViewedMovie.Create().SetJavdbID(id).SetViewedAt(now))
		}
		if err := tx.ViewedMovie.CreateBulk(builders...).
			OnConflictColumns(viewedmovie.FieldJavdbID).
			UpdateViewedAt().
			Exec(ctx); err != nil {
			return fmt.Errorf("upsert viewed movies: %w", err)
		}

		total, err := tx.ViewedMovie.Query().Count(ctx)
		if err != nil {
			return err
		}
		if total > maxViewedMovies {
			excess := total - maxViewedMovies
			oldestIDs, err := tx.ViewedMovie.Query().
				Order(ent.Asc(viewedmovie.FieldViewedAt), ent.Asc(viewedmovie.FieldID)).
				Limit(excess).
				IDs(ctx)
			if err != nil {
				return err
			}
			if len(oldestIDs) > 0 {
				if _, err := tx.ViewedMovie.Delete().Where(viewedmovie.IDIn(oldestIDs...)).Exec(ctx); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
