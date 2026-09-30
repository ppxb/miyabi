package maintenance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"entgo.io/ent/dialect/sql"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/tasks"
)

var ErrCacheBusy = domain.E(domain.KindBusy, "封面正在处理或缓存正在清理，请稍后重试", nil)

type Info struct {
	DataDirectory     string                `json:"data_directory"`
	DatabaseSizeBytes int64                 `json:"database_size_bytes"`
	Cache             mediaimage.CacheStats `json:"cache"`
}

// ArtworkLocker excludes pruning while new images gain durable references.
type ArtworkLocker interface {
	TryLockArtwork() bool
	UnlockArtwork()
}

type Service struct {
	directory string
	db        *ent.Client
	images    *mediaimage.Cache
	scrape    ArtworkLocker
}

func New(directory string, db *ent.Client, images *mediaimage.Cache, scrapeSvc ArtworkLocker) (*Service, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}
	return &Service{directory: absolute, db: db, images: images, scrape: scrapeSvc}, nil
}

func (service *Service) Info(ctx context.Context) (Info, error) {
	retained, err := service.retainedArtwork(ctx)
	if err != nil {
		return Info{}, err
	}
	return service.info(ctx, retained)
}

func (service *Service) ClearCache(ctx context.Context) (Info, error) {
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	if !service.scrape.TryLockArtwork() {
		return Info{}, ErrCacheBusy
	}
	defer service.scrape.UnlockArtwork()

	retained, err := service.retainedArtwork(ctx)
	if err != nil {
		return Info{}, err
	}
	if err := service.images.Prune(ctx, retained); err != nil {
		return Info{}, err
	}
	return service.info(ctx, retained)
}

func (service *Service) info(ctx context.Context, retained map[string]bool) (Info, error) {
	result := Info{DataDirectory: service.directory}
	var err error
	result.Cache, err = service.images.Stats(ctx, retained)
	if err != nil {
		return Info{}, err
	}
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		info, err := os.Stat(filepath.Join(service.directory, "miyabi.db"+suffix))
		if suffix != "" && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return Info{}, fmt.Errorf("read database size: %w", err)
		}
		if !info.Mode().IsRegular() {
			return Info{}, errors.New("database path is not a regular file")
		}
		result.DatabaseSizeBytes += info.Size()
	}
	return result, nil
}

func (service *Service) retainedArtwork(ctx context.Context) (map[string]bool, error) {
	// Read unfinished tasks before movies: cover completion publishes movie
	// references before marking its task done. This order cannot miss both sides
	// of that handoff, even when completion happens between the two queries.
	var pending []mediaimage.Artwork
	err := service.db.Task.Query().Where(
		task.TypeEQ(tasks.KindCover.String()), task.StatusNEQ(task.StatusDone),
		func(selector *sql.Selector) {
			selector.Select(
				"coalesce("+tasks.JSONExtract(task.FieldPayload, "artwork", "poster")+", '') AS poster",
				"coalesce("+tasks.JSONExtract(task.FieldPayload, "artwork", "fanart")+", '') AS fanart",
				"coalesce("+tasks.JSONExtract(task.FieldPayload, "artwork", "thumbnail")+", '') AS thumbnail",
			)
		},
	).Select(task.FieldID).Scan(ctx, &pending)
	if err != nil {
		return nil, fmt.Errorf("read pending artwork references: %w", err)
	}
	retained := make(map[string]bool)
	for _, artwork := range pending {
		retained[artwork.Poster] = true
		retained[artwork.Fanart] = true
		retained[artwork.Thumbnail] = true
	}
	// Include every account and directory, including films temporarily without
	// indexed files. Switching the active library must not make their covers disposable.
	records, err := service.db.Movie.Query().
		Select(movie.FieldID, movie.FieldCover, movie.FieldPoster, movie.FieldFanarts).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("read artwork references: %w", err)
	}
	for _, record := range records {
		retained[domain.ValueOrZero(record.Cover)] = true
		retained[domain.ValueOrZero(record.Poster)] = true
		for _, fanart := range record.Fanarts {
			retained[fanart] = true
		}
	}
	return retained, nil
}
