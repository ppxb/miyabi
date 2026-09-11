package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"entgo.io/ent/dialect/sql"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	mediaimage "github.com/ppxb/miyabi/internal/image"
)

var ErrCacheBusy = errors.New("封面正在处理或缓存正在清理，请稍后重试")

type DataInfo struct {
	DataDirectory     string                `json:"data_directory"`
	DatabaseSizeBytes int64                 `json:"database_size_bytes"`
	Cache             mediaimage.CacheStats `json:"cache"`
}

type DataService struct {
	directory string
	scrape    *ScrapeService
}

func NewDataService(directory string, scrape *ScrapeService) (*DataService, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}
	return &DataService{directory: absolute, scrape: scrape}, nil
}

func (service *DataService) Info(ctx context.Context) (DataInfo, error) {
	retained, err := service.retainedArtwork(ctx)
	if err != nil {
		return DataInfo{}, err
	}
	return service.info(ctx, retained)
}

func (service *DataService) ClearCache(ctx context.Context) (DataInfo, error) {
	if err := ctx.Err(); err != nil {
		return DataInfo{}, err
	}
	if !service.scrape.artwork.TryLock() {
		return DataInfo{}, ErrCacheBusy
	}
	defer service.scrape.artwork.Unlock()

	retained, err := service.retainedArtwork(ctx)
	if err != nil {
		return DataInfo{}, err
	}
	if err := service.scrape.images.Prune(ctx, retained); err != nil {
		return DataInfo{}, err
	}
	return service.info(ctx, retained)
}

func (service *DataService) info(ctx context.Context, retained map[string]bool) (DataInfo, error) {
	result := DataInfo{DataDirectory: service.directory}
	var err error
	result.Cache, err = service.scrape.images.Stats(ctx, retained)
	if err != nil {
		return DataInfo{}, err
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := ctx.Err(); err != nil {
			return DataInfo{}, err
		}
		info, err := os.Stat(filepath.Join(service.directory, "miyabi.db"+suffix))
		if suffix != "" && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return DataInfo{}, fmt.Errorf("read database size: %w", err)
		}
		if !info.Mode().IsRegular() {
			return DataInfo{}, errors.New("database path is not a regular file")
		}
		result.DatabaseSizeBytes += info.Size()
	}
	return result, nil
}

func (service *DataService) retainedArtwork(ctx context.Context) (map[string]bool, error) {
	// Include every account and directory, including films temporarily without
	// indexed files. Switching the active library must not make their covers disposable.
	records, err := service.scrape.library.database.Movie.Query().
		Select(movie.FieldID, movie.FieldCover, movie.FieldPoster, movie.FieldFanarts).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("read artwork references: %w", err)
	}
	retained := make(map[string]bool)
	for _, record := range records {
		retained[valueOrZero(record.Cover)] = true
		retained[valueOrZero(record.Poster)] = true
		for _, fanart := range record.Fanarts {
			retained[fanart] = true
		}
	}

	// Failed and queued cover jobs can resume from locally saved artwork before
	// the movie references it. Extract only those URLs, not the full NFO payloads.
	var pending []mediaimage.Artwork
	err = service.scrape.library.database.Task.Query().Where(
		task.TypeEQ("cover"), task.StatusNEQ(task.StatusDone),
		func(selector *sql.Selector) {
			selector.Select(
				"coalesce(json_extract(payload, '$.artwork.poster'), '') AS poster",
				"coalesce(json_extract(payload, '$.artwork.fanart'), '') AS fanart",
				"coalesce(json_extract(payload, '$.artwork.thumbnail'), '') AS thumbnail",
			)
		},
	).Select(task.FieldID).Scan(ctx, &pending)
	if err != nil {
		return nil, fmt.Errorf("read pending artwork references: %w", err)
	}
	for _, artwork := range pending {
		retained[artwork.Poster] = true
		retained[artwork.Fanart] = true
		retained[artwork.Thumbnail] = true
	}
	return retained, nil
}
