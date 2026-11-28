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
	"github.com/ppxb/miyabi/internal/ent/panaccount"
	"github.com/ppxb/miyabi/internal/ent/watchhistory"
	"github.com/ppxb/miyabi/internal/movieflags"
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
	settings := record.Settings
	return &domain.WatchResume{
		FileID:   record.FileID,
		Position: record.Position,
		Duration: record.Duration,
		Settings: settings,
	}, nil
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

	// Merge and upsert in one transaction so a concurrent save cannot interleave
	// between reading the stored settings and writing the merged blob.
	tx, err := service.database.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin watch progress transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var merged *domain.PlaybackSettings
	if settings != nil {
		existing, err := existingSettingsTx(ctx, tx, movieID)
		if err != nil {
			return err
		}
		merged = mergePlaybackSettings(existing, settings)
	}
	create := tx.WatchHistory.Create().
		SetMovieID(movieID).SetFileID(fileID).SetPosition(position).SetDuration(duration).SetWatchedAt(now)
	if merged != nil {
		create.SetSettings(merged)
	}
	if err := create.OnConflictColumns(watchhistory.FieldMovieID).
		Update(func(update *ent.WatchHistoryUpsert) {
			update.SetFileID(fileID).SetPosition(position).SetDuration(duration).SetWatchedAt(now)
			if merged != nil {
				update.SetSettings(merged)
			}
		}).Exec(ctx); err != nil {
		return fmt.Errorf("save watch progress: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit watch progress: %w", err)
	}
	return nil
}

// existingSettingsTx reads the stored settings blob for a movie within tx.
func existingSettingsTx(ctx context.Context, tx *ent.Tx, movieID int) (*domain.PlaybackSettings, error) {
	record, err := tx.WatchHistory.Query().
		Where(watchhistory.MovieIDEQ(movieID)).
		Select(watchhistory.FieldSettings).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read playback settings: %w", err)
	}
	return record.Settings, nil
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
	if incoming.Subtitle != "" {
		merged.Subtitle = incoming.Subtitle
	}
	if incoming.SubtitleOffset != nil {
		merged.SubtitleOffset = incoming.SubtitleOffset
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
	accounts, err := service.database.PanAccount.Query().
		Select(panaccount.FieldAccountID, panaccount.FieldProvider, panaccount.FieldDirectoryID).All(ctx)
	if err != nil {
		return result, fmt.Errorf("read history accounts: %w", err)
	}
	providers := make(map[string]string, len(accounts))
	mounted := make(map[[2]string]bool, len(accounts))
	for _, account := range accounts {
		provider := account.Provider
		if provider == "" {
			provider = "115"
		}
		providers[account.AccountID] = provider
		if account.DirectoryID != "" {
			mounted[[2]string{account.AccountID, account.DirectoryID}] = true
		}
	}
	flags, err := movieflags.Load(ctx, service.database, ids, providers, mounted)
	if err != nil {
		return result, err
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
		doc := domain.ValueOrZero(item.Metadata)
		result.Items = append(result.Items, domain.WatchHistoryItem{
			MovieID: item.ID, Code: item.Code, Title: item.Title, Cover: item.Cover, Poster: item.Poster,
			Rating: domain.NormalizeRating10(domain.ValueOrZero(item.Rating), doc.RatingMax),
			Actors: actors, Source: flags.Source[record.MovieID], HasSubtitle: flags.HasSubtitle[record.MovieID],
			WatchedAt: record.WatchedAt, Position: record.Position, Duration: record.Duration,
		})
	}
	result.HasMore = (page-1)*HistoryPageSize+len(result.Items) < result.Total
	return result, nil
}

// ClearHistory deletes all history entries.
func (service *Service) ClearHistory(ctx context.Context) (int, error) {
	count, err := service.database.WatchHistory.Delete().Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("clear watch history: %w", err)
	}
	return count, nil
}
