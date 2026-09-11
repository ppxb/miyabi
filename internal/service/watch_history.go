package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/predicate"
	"github.com/ppxb/miyabi/internal/ent/watchhistory"
)

const WatchHistoryPageSize = 20

var ErrWatchHistorySourceChanged = errors.New("媒体目录已切换，请刷新观看历史后重试")
var ErrInvalidWatchProgress = errors.New("观看进度无效")

type WatchHistoryScope struct {
	AccountID   string `json:"account_id" form:"account_id" binding:"required,max=128"`
	DirectoryID string `json:"directory_id" form:"directory_id" binding:"required,max=128"`
}

type WatchSession struct {
	ID        int     `json:"id"`
	SessionID string  `json:"session_id"`
	FileID    string  `json:"file_id"`
	Position  float64 `json:"position"`
	Duration  float64 `json:"duration"`
}

type WatchProgress struct {
	SessionID string  `json:"session_id" binding:"required,uuid"`
	FileID    string  `json:"file_id" binding:"required,max=128"`
	Position  float64 `json:"position" binding:"gte=0"`
	Duration  float64 `json:"duration" binding:"gt=0"`
	Version   int     `json:"version" binding:"min=1"`
}

type WatchHistoryItem struct {
	ID        int       `json:"id"`
	MovieID   int       `json:"movie_id"`
	Code      string    `json:"code"`
	Title     string    `json:"title"`
	Cover     *string   `json:"cover,omitempty"`
	Poster    *string   `json:"poster,omitempty"`
	WatchedAt time.Time `json:"watched_at"`
	Position  float64   `json:"position"`
	Duration  float64   `json:"duration"`
}

type WatchHistoryPage struct {
	Source  *LibrarySource     `json:"source,omitempty"`
	Items   []WatchHistoryItem `json:"items"`
	Total   int                `json:"total"`
	Page    int                `json:"page"`
	HasMore bool               `json:"has_more"`
}

func historyScope(source LibrarySource) predicate.WatchHistory {
	return watchhistory.And(watchhistory.AccountIDEQ(source.AccountID), watchhistory.RootIDEQ(source.Directory.ID))
}

// Opening a movie creates one source-scoped history entry and starts a fresh
// progress session. Reopening preserves its saved position and watched badge.
func (service *LibraryService) MarkWatched(ctx context.Context, movieID int) (WatchSession, error) {
	var result WatchSession
	libraryChanged := false
	err := ent.WithTx(ctx, service.database, func(tx *ent.Tx) error {
		source, err := loadLibrarySource(ctx, tx.Client())
		if err != nil {
			return err
		}
		if source == nil {
			return ErrMediaDirectoryRequired
		}
		record, err := tx.Movie.Query().Where(movie.IDEQ(movieID), movie.HasFilesWith(libraryFiles(*source))).
			Select(movie.FieldID, movie.FieldWatched).Only(ctx)
		if err != nil {
			return err
		}
		if !record.Watched {
			if err := tx.Movie.UpdateOneID(movieID).SetWatched(true).Exec(ctx); err != nil {
				return err
			}
			libraryChanged = true
		}
		if err := tx.WatchHistory.Create().SetAccountID(source.AccountID).SetRootID(source.Directory.ID).
			SetMovieID(movieID).SetWatchedAt(time.Now().UTC()).SetSessionID(uuid.NewString()).
			OnConflictColumns(watchhistory.FieldAccountID, watchhistory.FieldRootID, watchhistory.FieldMovieID).
			Update(func(update *ent.WatchHistoryUpsert) {
				update.UpdateWatchedAt().UpdateSessionID().SetProgressVersion(0)
			}).Exec(ctx); err != nil {
			return err
		}
		history, err := tx.WatchHistory.Query().Where(historyScope(*source), watchhistory.MovieIDEQ(movieID)).Only(ctx)
		if err != nil {
			return err
		}
		result = WatchSession{ID: history.ID, SessionID: history.SessionID, FileID: history.FileID,
			Position: history.Position, Duration: history.Duration}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("record library watch: %w", err)
	}
	if libraryChanged {
		service.tasks.NotifyLibraryChanged()
	} else {
		service.tasks.NotifyWatchHistoryChanged()
	}
	return result, nil
}

func (service *LibraryService) WatchHistory(ctx context.Context, page int) (WatchHistoryPage, error) {
	result := WatchHistoryPage{Items: []WatchHistoryItem{}, Page: page}
	source, err := loadLibrarySource(ctx, service.database)
	if err != nil || source == nil {
		return result, err
	}
	result.Source = source
	query := service.database.WatchHistory.Query().Where(historyScope(*source),
		watchhistory.HasMovieWith(movie.HasFilesWith(libraryFiles(*source))))
	result.Total, err = query.Clone().Count(ctx)
	if err != nil {
		return result, fmt.Errorf("count watch history: %w", err)
	}
	if result.Total == 0 {
		return result, nil
	}
	records, err := query.Select(watchhistory.FieldID, watchhistory.FieldMovieID, watchhistory.FieldWatchedAt,
		watchhistory.FieldPosition, watchhistory.FieldDuration).
		Order(ent.Desc(watchhistory.FieldWatchedAt), ent.Desc(watchhistory.FieldID)).
		Offset((page - 1) * WatchHistoryPageSize).Limit(WatchHistoryPageSize).
		WithMovie(func(query *ent.MovieQuery) {
			query.Select(movie.FieldID, movie.FieldCode, movie.FieldTitle, movie.FieldCover, movie.FieldPoster)
		}).All(ctx)
	if err != nil {
		return result, fmt.Errorf("list watch history: %w", err)
	}
	for _, record := range records {
		film, err := record.Edges.MovieOrErr()
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, WatchHistoryItem{
			ID: record.ID, MovieID: film.ID, Code: film.Code, Title: film.Title, Cover: film.Cover, Poster: film.Poster,
			WatchedAt: record.WatchedAt, Position: record.Position, Duration: record.Duration,
		})
	}
	result.HasMore = (page-1)*WatchHistoryPageSize+len(result.Items) < result.Total
	return result, nil
}

func (service *LibraryService) SaveWatchProgress(ctx context.Context, id int, progress WatchProgress) error {
	if progress.SessionID == "" || progress.FileID == "" || progress.Version < 1 || progress.Position < 0 ||
		progress.Duration <= 0 || math.IsNaN(progress.Position) || math.IsInf(progress.Position, 0) ||
		math.IsNaN(progress.Duration) || math.IsInf(progress.Duration, 0) {
		return ErrInvalidWatchProgress
	}
	changed := false
	err := ent.WithTx(ctx, service.database, func(tx *ent.Tx) error {
		source, err := loadLibrarySource(ctx, tx.Client())
		if err != nil {
			return err
		}
		if source == nil {
			return ErrMediaDirectoryRequired
		}
		record, err := tx.WatchHistory.Query().Where(historyScope(*source), watchhistory.IDEQ(id)).Only(ctx)
		if err != nil {
			return err
		}
		// A late request from another session or an older seek must not restore
		// stale progress. Updating existing rows also cannot recreate cleared history.
		if record.SessionID != progress.SessionID || record.ProgressVersion >= progress.Version {
			return nil
		}
		found, err := tx.File.Query().Where(libraryFiles(*source), file.MovieIDEQ(record.MovieID), file.FileIDEQ(progress.FileID)).Exist(ctx)
		if err != nil {
			return err
		}
		if !found {
			return fs.ErrNotExist
		}
		position := math.Min(progress.Position, progress.Duration)
		update := tx.WatchHistory.UpdateOneID(id).SetFileID(progress.FileID).SetPosition(position).
			SetDuration(progress.Duration).SetProgressVersion(progress.Version)
		changed = record.FileID != progress.FileID || record.Position != position || record.Duration != progress.Duration
		if changed {
			update.SetWatchedAt(time.Now().UTC())
		}
		return update.Exec(ctx)
	})
	if err != nil {
		return fmt.Errorf("save watch progress: %w", err)
	}
	if changed {
		service.tasks.NotifyWatchHistoryChanged()
	}
	return nil
}

func (service *LibraryService) RemoveWatchHistory(ctx context.Context, scope WatchHistoryScope, ids []int) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	return service.deleteWatchHistory(ctx, scope, watchhistory.IDIn(ids...))
}

func (service *LibraryService) ClearWatchHistory(ctx context.Context, scope WatchHistoryScope) (int, error) {
	return service.deleteWatchHistory(ctx, scope)
}

func (service *LibraryService) deleteWatchHistory(ctx context.Context, scope WatchHistoryScope, filters ...predicate.WatchHistory) (int, error) {
	count := 0
	err := ent.WithTx(ctx, service.database, func(tx *ent.Tx) error {
		source, err := loadLibrarySource(ctx, tx.Client())
		if err != nil {
			return err
		}
		if source == nil || source.AccountID != scope.AccountID || source.Directory.ID != scope.DirectoryID {
			return ErrWatchHistorySourceChanged
		}
		count, err = tx.WatchHistory.Delete().Where(historyScope(*source)).Where(filters...).Exec(ctx)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("clear watch history: %w", err)
	}
	if count > 0 {
		service.tasks.NotifyWatchHistoryChanged()
	}
	return count, nil
}
