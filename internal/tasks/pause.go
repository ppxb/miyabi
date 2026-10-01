package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/setting"
)

const libraryPauseKey = "tasks.library_paused"

// ErrPaused yields a handler after its durable checkpoint, without spending a retry.
var ErrPaused = errors.New("library processing paused")

func LibraryPaused(ctx context.Context, db *ent.Client) (bool, error) {
	value, err := db.Setting.Query().Where(setting.KeyEQ(libraryPauseKey)).Only(ctx)
	if ent.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var paused bool
	err = json.Unmarshal(value.Value, &paused)
	return paused, err
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
	value, _ := json.Marshal(paused)
	if err := s.queue.database.Setting.Create().SetKey(libraryPauseKey).SetValue(value).
		OnConflictColumns(setting.FieldKey).UpdateNewValues().Exec(ctx); err != nil {
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
