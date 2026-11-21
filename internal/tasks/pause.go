package tasks

import (
	"context"
	"errors"
	"slices"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent"
)

const libraryPauseKey = "tasks.library_paused"

// ErrPaused yields a handler after its durable checkpoint, without spending a retry.
var ErrPaused = errors.New("library processing paused")

func LibraryPaused(ctx context.Context, db *ent.Client) (bool, error) {
	value, _, err := database.LoadSetting[bool](ctx, db, libraryPauseKey)
	if err != nil {
		return false, err
	}
	return value, nil
}

func Checkpoint(ctx context.Context, db *ent.Client) error {
	paused, err := LibraryPaused(ctx, db)
	if err != nil {
		return err
	}
	if paused {
		return ErrPaused
	}
	return nil
}

func (s *Service) SetLibraryPaused(ctx context.Context, paused bool) error {
	if err := s.queue.Lock(ctx); err != nil {
		return err
	}
	defer s.queue.Unlock()
	if err := database.SaveSetting(ctx, s.queue.database, libraryPauseKey, paused); err != nil {
		return err
	}
	s.NotifyUI()
	s.WakePool()
	return nil
}

func (q *Queue) runnableKinds(ctx context.Context, kinds []Kind) ([]Kind, error) {
	isLibrary := func(kind Kind) bool { return kind == KindScan || kind == KindScrape }
	if !slices.ContainsFunc(kinds, isLibrary) {
		return kinds, nil
	}
	paused, err := LibraryPaused(ctx, q.database)
	if err != nil {
		return nil, err
	}
	if paused {
		kinds = slices.DeleteFunc(slices.Clone(kinds), isLibrary)
	}
	return kinds, nil
}
