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
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/tasks"
)

// ReconcileScan executes reconciliation within a fresh transaction.
func ReconcileScan(ctx context.Context, db *ent.Client, taskID int, scanID string, payload *Payload, observed scrape.DirectoryObservations, images *mediaimage.Cache, tasksSvc *tasks.Service, embyOpts ...any) error {
	return ent.WithTx(ctx, db, func(tx *ent.Tx) error {
		return ReconcileScanTx(ctx, tx, taskID, scanID, payload, observed, images, tasksSvc, embyOpts...)
	})
}

// ReconcileScanTx cleans up missing files, updates offline workflows, and schedules metadata scrape tasks.
func ReconcileScanTx(ctx context.Context, tx *ent.Tx, taskID int, scanID string, payload *Payload, observed scrape.DirectoryObservations, images *mediaimage.Cache, tasksSvc *tasks.Service, embyOpts ...any) error {
	embyDir, publicURL, strmToken := "", "", ""
	var notifier scrape.MediaNotifier
	var strCount int
	for _, opt := range embyOpts {
		switch v := opt.(type) {
		case string:
			switch strCount {
			case 0:
				embyDir = v
			case 1:
				publicURL = v
			case 2:
				strmToken = v
			}
			strCount++
		case scrape.MediaNotifier:
			notifier = v
		}
	}
	stale := file.And(database.LibraryFiles(payload.Source), file.ScanIDNEQ(scanID))
	if payload.TargetID != "" {
		if payload.TargetFile {
			stale = file.And(stale, file.FileIDEQ(payload.TargetID))
		} else {
			prefix := strings.TrimSuffix(payload.TargetPath, "/") + "/"
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
	payload.Scan.RemovedFiles, err = tx.File.Delete().Where(stale).Exec(ctx)
	if err != nil {
		return fmt.Errorf("remove missing file indexes: %w", err)
	}
	removed, err := RemoveUnreferencedMovies(ctx, tx, movies)
	if err != nil {
		return err
	}
	payload.Scan.RemovedMovies += len(removed)
	if embyDir != "" && len(removed) > 0 {
		tx.OnCommit(func(next ent.Committer) ent.Committer {
			return ent.CommitFunc(func(ctx context.Context, tx *ent.Tx) error {
				if err := next.Commit(ctx, tx); err != nil {
					return err
				}
				var cleanupErrors []error
				for _, code := range removed {
					movieDir := scrape.EmbyMovieDir(embyDir, code)
					if err := os.RemoveAll(movieDir); err != nil {
						cleanupErrors = append(cleanupErrors, fmt.Errorf("remove exported movie %s after commit: %w", code, err))
						continue
					}
					_ = os.Remove(filepath.Dir(movieDir)) // Keep non-empty prefix directories.
					if notifier != nil {
						notifier.NotifyUpdated(movieDir)
					}
				}
				return errors.Join(cleanupErrors...)
			})
		})
	}
	indexed := file.And(database.LibraryFiles(payload.Source), file.ScanIDEQ(scanID))
	if payload.OfflineTaskID != 0 {
		files, err := tx.File.Query().Where(indexed).Select(file.FieldFileID).All(ctx)
		if err != nil {
			return err
		}
		record, err := tx.Task.Get(ctx, payload.OfflineTaskID)
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(files))
		for _, entry := range files {
			ids = append(ids, entry.FileID)
		}
		record.Payload, err = tasks.SetPayloadField(record.Payload, "file_ids", ids)
		if err != nil {
			return err
		}
		if err := tx.Task.UpdateOneID(record.ID).SetPayload(record.Payload).Exec(ctx); err != nil {
			return err
		}
	}
	moviesToScrape, err := tx.File.Query().Where(indexed).QueryMovie().
		WithFiles(func(q *ent.FileQuery) { q.Where(database.LibraryFiles(payload.Source)) }).
		WithActors().WithTags().All(ctx)
	if err != nil {
		return fmt.Errorf("find scanned metadata jobs: %w", err)
	}
	ids := make([]int, 0, len(moviesToScrape))
	for _, record := range moviesToScrape {
		if record.ScrapeStatus == movie.ScrapeStatusDone {
			ids = append(ids, record.ID)
		}
	}
	snapshots, err := scrape.CompletedMetadataSnapshots(ctx, tx.Client(), payload.Source, ids)
	if err != nil {
		return err
	}
	for _, record := range moviesToScrape {
		if snapshot, found := snapshots[record.ID]; found && record.ScrapeStatus == movie.ScrapeStatusDone && snapshot.Matches(record, observed) {
			if images != nil {
				cached, err := images.Exists(scrape.MovieArtwork(record))
				if err != nil {
					return fmt.Errorf("check cached artwork: %w", err)
				}
				if cached {
					if embyDir != "" {
						if err := scrape.ExportLocalMovie(embyDir, publicURL, strmToken, record, images, notifier); err != nil {
							return fmt.Errorf("export local movie %s: %w", record.Code, err)
						}
					}
					continue
				}
			}
		}

		input := scrape.MetadataPayload{
			Source:     payload.Source,
			ScanTaskID: taskID,
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
	payload.Scan.Stage = "done"
	if err := SaveScanProgress(ctx, tx.Task, taskID, *payload); err != nil {
		return err
	}
	if tasksSvc != nil {
		if payload.Scan.RemovedFiles > 0 || payload.Scan.RemovedMovies > 0 {
			tasksSvc.NotifyLibraryChanged()
		} else {
			tasksSvc.Notify()
		}
	}
	return nil
}
