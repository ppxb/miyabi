package playback

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	subtitlemeta "github.com/ppxb/miyabi/internal/domain/subtitle"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/subtitle"
	"github.com/ppxb/miyabi/internal/ent/watchhistory"
)

const HistoryPageSize = 20

// Resume returns the last saved progress for a movie, if any.
func (service *Service) Resume(ctx context.Context, movieID int) (*domain.WatchResume, error) {
	record, err := service.database.WatchHistory.Query().
		Where(watchhistory.MovieIDEQ(movieID)).
		Select(watchhistory.FieldFileID, watchhistory.FieldPosition, watchhistory.FieldDuration, watchhistory.FieldSettings).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read watch history: %w", err)
	}
	settings, err := decodePlaybackSettings(record.Settings)
	if err != nil {
		return nil, err
	}
	return &domain.WatchResume{
		FileID:   record.FileID,
		Position: record.Position,
		Duration: record.Duration,
		Settings: settings,
	}, nil
}

// decodePlaybackSettings unmarshals the JSON settings blob, returning nil when
// it is absent so callers fall back to defaults.
func decodePlaybackSettings(raw json.RawMessage) (*domain.PlaybackSettings, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var settings domain.PlaybackSettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return nil, fmt.Errorf("decode playback settings: %w", err)
	}
	return &settings, nil
}

// SaveProgress validates the file and upserts the movie's playback progress and
// the extensible playback settings (speed, rendition, ...). A nil settings keeps
// whatever was stored before.
func (service *Service) SaveProgress(ctx context.Context, movieID int, fileID string, position, duration float64, settings *domain.PlaybackSettings) error {
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
	var rawSettings json.RawMessage
	if settings != nil {
		existing, err := service.existingSettings(ctx, movieID)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(mergePlaybackSettings(existing, settings))
		if err != nil {
			return fmt.Errorf("encode playback settings: %w", err)
		}
		rawSettings = encoded
	}
	create := service.database.WatchHistory.Create().
		SetMovieID(movieID).SetFileID(fileID).SetPosition(position).SetDuration(duration).SetWatchedAt(now)
	if rawSettings != nil {
		create.SetSettings(rawSettings)
	}
	if err := create.OnConflictColumns(watchhistory.FieldMovieID).
		Update(func(update *ent.WatchHistoryUpsert) {
			update.SetFileID(fileID).SetPosition(position).SetDuration(duration).SetWatchedAt(now)
			if rawSettings != nil {
				update.SetSettings(rawSettings)
			}
		}).Exec(ctx); err != nil {
		return fmt.Errorf("save watch progress: %w", err)
	}
	return nil
}

// existingSettings reads the stored settings blob for a movie, if any.
func (service *Service) existingSettings(ctx context.Context, movieID int) (*domain.PlaybackSettings, error) {
	record, err := service.database.WatchHistory.Query().
		Where(watchhistory.MovieIDEQ(movieID)).
		Select(watchhistory.FieldSettings).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read playback settings: %w", err)
	}
	return decodePlaybackSettings(record.Settings)
}

// mergePlaybackSettings overlays the provided fields onto the stored ones so a
// partial update never clears a preference it did not mention. This keeps the
// single settings blob additive as new preferences (audio tracks, subtitles,
// ...) are introduced.
func mergePlaybackSettings(base, incoming *domain.PlaybackSettings) *domain.PlaybackSettings {
	if base == nil {
		return incoming
	}
	merged := *base
	if incoming.Rate > 0 {
		merged.Rate = incoming.Rate
	}
	if incoming.Quality != "" {
		merged.Quality = incoming.Quality
	}
	return &merged
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
		Select(movie.FieldID, movie.FieldCode, movie.FieldTitle, movie.FieldCover, movie.FieldPoster, movie.FieldRating, movie.FieldMetadata).
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
	historyFiles, err := service.database.File.Query().
		Where(file.MovieIDIn(ids...)).
		Select(file.FieldMovieID, file.FieldName, file.FieldAccountID).All(ctx)
	if err != nil {
		return result, fmt.Errorf("read history files: %w", err)
	}
	cloud := make(map[int]bool, len(historyFiles))
	subtitled := make(map[int]bool, len(historyFiles))
	for _, record := range historyFiles {
		if record.MovieID == nil {
			continue
		}
		id := *record.MovieID
		if record.AccountID != "" && record.AccountID != domain.LocalAccountID {
			cloud[id] = true
		}
		if subtitlemeta.HasChineseSubtitle(record.Name) {
			subtitled[id] = true
		}
	}
	tracks, err := service.database.Subtitle.Query().
		Where(subtitle.MovieIDIn(ids...)).
		Select(subtitle.FieldMovieID, subtitle.FieldName).All(ctx)
	if err != nil {
		return result, fmt.Errorf("read history subtitles: %w", err)
	}
	for _, track := range tracks {
		if subtitlemeta.HasChineseSubtitle(track.Name) {
			subtitled[track.MovieID] = true
		}
	}
	for _, record := range records {
		item, ok := byID[record.MovieID]
		if !ok {
			continue
		}
		source := ""
		if cloud[record.MovieID] {
			source = "115"
		}
		actors := make([]domain.Actor, 0, len(item.Edges.Actors))
		for _, person := range item.Edges.Actors {
			actors = append(actors, domain.Actor{Provider: person.Provider, ID: person.SourceID, Name: person.Name})
		}
		doc := domain.ValueOrZero(item.Metadata)
		result.Items = append(result.Items, domain.WatchHistoryItem{
			MovieID: item.ID, Code: item.Code, Title: item.Title, Cover: item.Cover, Poster: item.Poster,
			Rating: domain.NormalizeRating10(domain.ValueOrZero(item.Rating), doc.RatingMax),
			Actors: actors, Source: source, HasSubtitle: subtitled[record.MovieID],
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
