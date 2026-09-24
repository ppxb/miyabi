package scan

import (
	"context"
	"fmt"
	"maps"
	"path"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/tasks"
)

type offlineTaskPayload struct {
	FileIDs []string `json:"file_ids,omitempty"`
}

// ProcessScanPage persists file associations and movie records for a scanned page within a transaction.
func ProcessScanPage(ctx context.Context, db *ent.Client, taskID int, scanID, directoryPath string, videos []Video, payload *Payload, prepare func([]Video) []Video, tasksSvc *tasks.Service) error {
	return ent.WithTx(ctx, db, func(tx *ent.Tx) error {
		return ProcessScanPageTx(ctx, tx, taskID, scanID, directoryPath, videos, payload, prepare, tasksSvc)
	})
}

// ProcessScanPageTx reads identities and writes file associations in the same transaction.
// prepare accounts for identified videos and can filter or defer unknown files.
func ProcessScanPageTx(ctx context.Context, tx *ent.Tx, taskID int, scanID, directoryPath string, videos []Video, payload *Payload, prepare func([]Video) []Video, tasksSvc *tasks.Service) error {
	indexChanged := false
	offlineChanged := false

	if len(videos) > 0 {
		ids := make([]string, 0, len(videos))
		for _, video := range videos {
			ids = append(ids, video.ID)
		}
		previous, err := tx.File.Query().Where(file.FileIDIn(ids...)).
			Select(file.FieldID, file.FieldFileID, file.FieldName, file.FieldParentID, file.FieldSize,
				file.FieldSha1, file.FieldPickCode, file.FieldAccountID, file.FieldRootID, file.FieldPath, file.FieldMovieID).
			WithMovie(func(q *ent.MovieQuery) { q.Select(movie.FieldID, movie.FieldCode, movie.FieldJavdbID) }).All(ctx)
		if err != nil {
			return fmt.Errorf("load previous file associations: %w", err)
		}
		previousFiles := make(map[string]*ent.File, len(previous))
		for _, entry := range previous {
			previousFiles[entry.FileID] = entry
		}
		if prepare != nil {
			IdentifyScanVideos(*payload, videos, previousFiles)
			videos = prepare(videos)
		}
		ids = ids[:0]
		codes := make(map[string]int)
		for _, video := range videos {
			ids = append(ids, video.ID)
			if video.Code != "" {
				codes[video.Code] = 0
			}
		}
		for _, entry := range previous {
			// A scraped record with the exact number already outranks every equivalent match.
			if film := entry.Edges.Movie; film != nil && film.JavdbID != nil {
				if _, needed := codes[film.Code]; needed {
					codes[film.Code] = film.ID
				}
			}
		}
		if len(codes) > 0 && payload.OfflineTaskID != 0 && payload.TargetID != "" {
			id, err := IndexDownloadedMovie(ctx, tx, *payload)
			if err != nil {
				return err
			}
			for code := range codes {
				codes[code] = id
			}
		} else if len(codes) > 0 {
			var numbers []string
			for code, id := range codes {
				if id == 0 {
					numbers = append(numbers, code)
				}
			}
			indexed, err := indexMovies(ctx, tx, numbers)
			if err != nil {
				return err
			}
			maps.Copy(codes, indexed)
		}
		builders := make([]*ent.FileCreate, 0, len(videos))
		var unchanged []string
		var previousMovies []int
		for _, video := range videos {
			old := previousFiles[video.ID]
			if old != nil && old.Name == video.Name && old.ParentID == video.ParentID &&
				old.Size == video.Size && old.Sha1 == video.SHA1 && old.PickCode == video.PickCode &&
				old.AccountID == payload.Source.AccountID && old.RootID == payload.Source.Directory.ID &&
				old.Path == path.Join(directoryPath, video.Name) && domain.ValueOrZero(old.MovieID) == codes[video.Code] {
				unchanged = append(unchanged, video.ID)
				continue
			}
			indexChanged = true
			if old != nil && old.MovieID != nil && *old.MovieID != codes[video.Code] {
				previousMovies = append(previousMovies, *old.MovieID)
			}
			builder := tx.File.Create().SetFileID(video.ID).SetName(video.Name).SetSize(video.Size).
				SetPickCode(video.PickCode).SetSha1(video.SHA1).SetParentID(video.ParentID).
				SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).
				SetPath(path.Join(directoryPath, video.Name)).SetScanID(scanID)
			if video.Code != "" {
				builder.SetMovieID(codes[video.Code])
			}
			builders = append(builders, builder)
		}
		if len(builders) > 0 {
			if err := tx.File.CreateBulk(builders...).OnConflictColumns(file.FieldFileID).
				UpdateNewValues().UpdateMovieID().Exec(ctx); err != nil {
				return fmt.Errorf("index scanned files: %w", err)
			}
		}
		if len(unchanged) > 0 {
			if err := tx.File.Update().Where(file.FileIDIn(unchanged...)).SetScanID(scanID).Exec(ctx); err != nil {
				return fmt.Errorf("mark unchanged scanned files: %w", err)
			}
		}
		removed, err := RemoveUnreferencedMovies(ctx, tx, previousMovies)
		if err != nil {
			return err
		}
		payload.Scan.RemovedMovies += removed
		if len(videos) > 0 && payload.OfflineTaskID != 0 {
			record, err := tx.Task.Get(ctx, payload.OfflineTaskID)
			if err != nil {
				return err
			}
			input, err := tasks.DecodePayload[offlineTaskPayload](record.Payload)
			if err != nil {
				return err
			}
			known := make(map[string]bool, len(input.FileIDs))
			for _, id := range input.FileIDs {
				known[id] = true
			}
			for _, id := range ids {
				if !known[id] {
					input.FileIDs = append(input.FileIDs, id)
					known[id], offlineChanged = true, true
				}
			}
			if offlineChanged {
				record.Payload, err = tasks.SetPayloadField(record.Payload, "file_ids", input.FileIDs)
				if err != nil {
					return err
				}
				if err := tx.Task.UpdateOneID(record.ID).SetPayload(record.Payload).Exec(ctx); err != nil {
					return err
				}
			}
		}
	}
	if err := SaveScanProgress(ctx, tx.Task, taskID, *payload); err != nil {
		return err
	}
	if tasksSvc != nil {
		if indexChanged {
			tasksSvc.NotifyLibraryChanged()
		} else if offlineChanged {
			tasksSvc.NotifyOfflineChanged()
		} else {
			tasksSvc.Notify()
		}
	}
	return nil
}

// IndexDownloadedMovie binds a completed download target to a movie record with its known JavDB ID.
func IndexDownloadedMovie(ctx context.Context, tx *ent.Tx, payload Payload) (int, error) {
	code := codeid.Normalize(payload.Code)
	if err := tx.Movie.Create().SetCode(code).SetJavdbID(payload.JavDBID).
		OnConflict().Ignore().Exec(ctx); err != nil {
		return 0, fmt.Errorf("index downloaded movie: %w", err)
	}
	record, err := tx.Movie.Query().Where(movie.Or(movie.JavdbIDEQ(payload.JavDBID),
		movie.And(movie.CodeEQ(code), movie.JavdbIDIsNil()))).Only(ctx)
	if err != nil {
		return 0, fmt.Errorf("resolve downloaded movie association: %w", err)
	}
	if record.JavdbID == nil {
		if err := tx.Movie.UpdateOneID(record.ID).SetJavdbID(payload.JavDBID).Exec(ctx); err != nil {
			return 0, fmt.Errorf("save downloaded movie identity: %w", err)
		}
	}
	return record.ID, nil
}

// RemoveUnreferencedMovies deletes movie records that no longer have any associated media files.
func RemoveUnreferencedMovies(ctx context.Context, tx *ent.Tx, ids []int) (int, error) {
	removed := 0
	for start := 0; start < len(ids); start += 500 {
		count, err := tx.Movie.Delete().Where(
			movie.IDIn(ids[start:min(start+500, len(ids))]...), movie.Not(movie.HasFiles()),
		).Exec(ctx)
		if err != nil {
			return 0, fmt.Errorf("remove movies without files: %w", err)
		}
		removed += count
	}
	return removed, nil
}

// SaveScanProgress updates a scan task record's payload with latest progress.
func SaveScanProgress(ctx context.Context, client *ent.TaskClient, taskID int, payload Payload) error {
	encoded, err := tasks.EncodePayload(payload)
	if err != nil {
		return err
	}
	if err := client.UpdateOneID(taskID).SetPayload(encoded).Exec(ctx); err != nil {
		return fmt.Errorf("save scan progress: %w", err)
	}
	return nil
}

// ReportScan saves progress and notifies the task bus.
func ReportScan(ctx context.Context, client *ent.TaskClient, taskID int, payload Payload, tasksSvc *tasks.Service) error {
	if err := SaveScanProgress(ctx, client, taskID, payload); err != nil {
		return err
	}
	if tasksSvc != nil {
		tasksSvc.Notify()
	}
	return nil
}
