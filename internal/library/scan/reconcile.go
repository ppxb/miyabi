package scan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"entgo.io/ent/dialect/sql"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/export"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/tasks"
)

// ReconcileScan executes reconciliation within a fresh transaction.
func ReconcileScan(ctx context.Context, db *ent.Client, taskID int, payload *Payload, images *mediaimage.Cache, tasksSvc *tasks.Service, cfg export.Config, notifier scrape.MediaNotifier) error {
	run := scanRun{scanner: &Scanner{images: images, tasksSvc: tasksSvc, notifier: notifier}, taskID: taskID, payload: payload}
	return ent.WithTx(ctx, db, func(tx *ent.Tx) error { return run.reconcileTx(ctx, tx, cfg) })
}

// reconcileTx cleans up missing files, updates offline workflows, and schedules metadata scrape tasks.
func (r *scanRun) reconcileTx(ctx context.Context, tx *ent.Tx, cfg export.Config) error {
	stale := file.And(database.LibraryFiles(r.payload.Source), file.ScanIDNEQ(r.payload.ScanID))
	if r.payload.TargetID != "" {
		if r.payload.TargetFile {
			stale = file.And(stale, file.FileIDEQ(r.payload.TargetID))
		} else {
			prefix := strings.TrimSuffix(r.payload.TargetPath, "/") + "/"
			stale = file.And(stale, func(s *sql.Selector) {
				s.Where(sql.ExprP("substr("+s.C(file.FieldPath)+", 1, length(?)) = ?", prefix, prefix))
			})
		}
	}
	var err error
	movies, err := tx.File.Query().Where(stale).QueryMovie().IDs(ctx)
	if err != nil {
		return fmt.Errorf("find removed movie files: %w", err)
	}
	r.payload.Scan.RemovedFiles, err = tx.File.Delete().Where(stale).Exec(ctx)
	if err != nil {
		return fmt.Errorf("remove missing file indexes: %w", err)
	}
	removed, err := RemoveUnreferencedMovies(ctx, tx, movies)
	if err != nil {
		return err
	}
	r.payload.Scan.RemovedMovies += len(removed)
	if cfg.EmbyDir != "" && len(removed) > 0 {
		tx.OnCommit(func(next ent.Committer) ent.Committer {
			return ent.CommitFunc(func(ctx context.Context, tx *ent.Tx) error {
				if err := next.Commit(ctx, tx); err != nil {
					return err
				}
				var cleanupErrors []error
				for _, code := range removed {
					movieDir := scrape.EmbyMovieDir(cfg.EmbyDir, code)
					if err := os.RemoveAll(movieDir); err != nil {
						cleanupErrors = append(cleanupErrors, fmt.Errorf("remove exported movie %s after commit: %w", code, err))
						continue
					}
					_ = os.Remove(filepath.Dir(movieDir)) // Keep non-empty prefix directories.
					if r.scanner.notifier != nil {
						r.scanner.notifier.NotifyUpdated(movieDir)
					}
				}
				return errors.Join(cleanupErrors...)
			})
		})
	}
	indexed := file.And(database.LibraryFiles(r.payload.Source), file.ScanIDEQ(r.payload.ScanID))
	if r.payload.OfflineTaskID != 0 {
		files, err := tx.File.Query().Where(indexed).Select(file.FieldFileID).All(ctx)
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(files))
		for _, entry := range files {
			ids = append(ids, entry.FileID)
		}
		if err := tx.OfflineDownload.UpdateOneID(r.payload.OfflineTaskID).SetFileIds(ids).Exec(ctx); err != nil {
			return err
		}
	}
	moviesToScrape, err := tx.File.Query().Where(indexed).QueryMovie().
		WithFiles(func(q *ent.FileQuery) { q.Where(database.LibraryFiles(r.payload.Source)) }).
		WithActors().WithTags().All(ctx)
	if err != nil {
		return fmt.Errorf("find scanned metadata jobs: %w", err)
	}
	for _, record := range moviesToScrape {
		if record.ScrapeStatus == movie.ScrapeStatusDone && scrape.SnapshotMatches(record, r.payload.Source) {
			if r.scanner.images != nil {
				cached, err := r.scanner.images.Exists(scrape.MovieArtwork(record))
				if err != nil {
					return fmt.Errorf("check cached artwork: %w", err)
				}
				if cached {
					if cfg.EmbyDir != "" {
						if err := scrape.ExportLocalMovie(cfg.EmbyDir, cfg.PublicURL, cfg.STRMToken, record, r.scanner.images, r.scanner.notifier); err != nil {
							return fmt.Errorf("export local movie %s: %w", record.Code, err)
						}
					}
					continue
				}
			}
		}

		input := scrape.MetadataPayload{
			Source:     r.payload.Source,
			ScanTaskID: r.taskID,
			MovieID:    record.ID,
			Code:       record.Code,
			JavDBID:    domain.ValueOrZero(record.JavdbID),
		}
		encoded, err := tasks.EncodePayload(input)
		if err != nil {
			return err
		}
		if err := tx.Task.Create().SetType(tasks.KindScrape.String()).SetPayload(encoded).Exec(ctx); err != nil {
			return fmt.Errorf("enqueue movie metadata: %w", err)
		}
	}
	r.payload.Scan.Stage = "done"
	if err := SaveScanProgress(ctx, tx.Task, r.taskID, *r.payload); err != nil {
		return err
	}
	if r.scanner.tasksSvc != nil {
		if r.payload.Scan.RemovedFiles > 0 || r.payload.Scan.RemovedMovies > 0 {
			r.scanner.tasksSvc.NotifyLibraryChanged()
		} else {
			r.scanner.tasksSvc.Notify()
		}
	}
	return nil
}
