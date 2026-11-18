package playback

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/watchhistory"
)

const HistoryPageSize = 20

// Resume returns the last saved progress for a movie, if any.
func (service *Service) Resume(ctx context.Context, movieID int) (*domain.WatchResume, error) {
	record, err := service.database.WatchHistory.Query().
		Where(watchhistory.MovieIDEQ(movieID)).
		Select(watchhistory.FieldFileID, watchhistory.FieldPosition, watchhistory.FieldDuration).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read watch history: %w", err)
	}
	return &domain.WatchResume{FileID: record.FileID, Position: record.Position, Duration: record.Duration}, nil
}

// SaveProgress validates the file and upserts the movie's playback progress.
func (service *Service) SaveProgress(ctx context.Context, movieID int, fileID string, position, duration float64) error {
	if movieID <= 0 || fileID == "" || position < 0 || duration <= 0 ||
		math.IsNaN(position) || math.IsInf(position, 0) || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return domain.E(domain.KindInvalid, "观看进度无效", nil)
	}
	found, err := service.database.File.Query().
		Where(file.FileIDEQ(fileID), file.MovieIDEQ(movieID)).Exist(ctx)
	if err != nil {
		return fmt.Errorf("check playback file: %w", err)
	}
	if !found {
		return domain.E(domain.KindNotFound, "播放文件不存在", nil)
	}
	position = math.Min(position, duration)
	now := time.Now().UTC()
	if err := service.database.WatchHistory.Create().
		SetMovieID(movieID).SetFileID(fileID).SetPosition(position).SetDuration(duration).SetWatchedAt(now).
		OnConflictColumns(watchhistory.FieldMovieID).
		Update(func(update *ent.WatchHistoryUpsert) {
			update.SetFileID(fileID).SetPosition(position).SetDuration(duration).SetWatchedAt(now)
		}).Exec(ctx); err != nil {
		return fmt.Errorf("save watch progress: %w", err)
	}
	return nil
}

// History lists watched movies newest first.
func (service *Service) History(ctx context.Context, page int) (domain.WatchHistoryPage, error) {
	result := domain.WatchHistoryPage{Items: []domain.WatchHistoryItem{}, Page: page}
	query := service.database.WatchHistory.Query()
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return result, fmt.Errorf("count watch history: %w", err)
	}
	result.Total = total
	if total == 0 {
		return result, nil
	}
	records, err := query.Order(ent.Desc(watchhistory.FieldWatchedAt), ent.Desc(watchhistory.FieldID)).
		Offset((page - 1) * HistoryPageSize).Limit(HistoryPageSize).All(ctx)
	if err != nil {
		return result, fmt.Errorf("list watch history: %w", err)
	}
	ids := make([]int, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.MovieID)
	}
	movies, err := service.database.Movie.Query().Where(movie.IDIn(ids...)).
		Select(movie.FieldID, movie.FieldCode, movie.FieldTitle, movie.FieldCover, movie.FieldPoster, movie.FieldRating).
		WithActors(func(query *ent.ActorQuery) {
			query.Select(actor.FieldProvider, actor.FieldSourceID, actor.FieldName)
		}).All(ctx)
	if err != nil {
		return result, fmt.Errorf("read history movies: %w", err)
	}
	byID := make(map[int]*ent.Movie, len(movies))
	for _, item := range movies {
		byID[item.ID] = item
	}
	for _, record := range records {
		item, ok := byID[record.MovieID]
		if !ok {
			continue
		}
		actors := make([]domain.Actor, 0, len(item.Edges.Actors))
		for _, person := range item.Edges.Actors {
			actors = append(actors, domain.Actor{Provider: person.Provider, ID: person.SourceID, Name: person.Name})
		}
		result.Items = append(result.Items, domain.WatchHistoryItem{
			MovieID: item.ID, Code: item.Code, Title: item.Title, Cover: item.Cover, Poster: item.Poster,
			Rating: domain.ValueOrZero(item.Rating), Actors: actors,
			WatchedAt: record.WatchedAt, Position: record.Position, Duration: record.Duration,
		})
	}
	result.HasMore = (page-1)*HistoryPageSize+len(result.Items) < result.Total
	return result, nil
}

// RemoveHistory deletes selected history entries.
func (service *Service) RemoveHistory(ctx context.Context, ids []int) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	count, err := service.database.WatchHistory.Delete().Where(watchhistory.IDIn(ids...)).Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("remove watch history: %w", err)
	}
	return count, nil
}

// ClearHistory deletes all history entries.
func (service *Service) ClearHistory(ctx context.Context) (int, error) {
	count, err := service.database.WatchHistory.Delete().Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("clear watch history: %w", err)
	}
	return count, nil
}
