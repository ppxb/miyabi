package maintenance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/logging"
	"github.com/ppxb/miyabi/internal/tasks"
)

var ErrCacheBusy = domain.E(domain.KindBusy, "封面正在处理或缓存正在清理，请稍后重试", nil)

// retainedTTL bounds how stale the display-only "unused" cache stats may be.
// Pruning never uses this cache; ClearCache always recomputes references.
const retainedTTL = 30 * time.Minute

type Info struct {
	DataDirectory     string                `json:"data_directory"`
	DatabaseSizeBytes int64                 `json:"database_size_bytes"`
	Cache             mediaimage.CacheStats `json:"cache"`
}

// LogInfo is the tail of the runtime log file.
type LogInfo struct {
	Path      string   `json:"path"`
	Lines     []string `json:"lines"`
	SizeBytes int64    `json:"size_bytes"`
}

// Logs reads the newest lines of the runtime log without loading the whole file.
func (service *Service) Logs(ctx context.Context, limit int) (LogInfo, error) {
	path := logging.Path(service.directory)
	result := LogInfo{Path: path, Lines: []string{}}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("stat runtime log: %w", err)
	}
	if !info.Mode().IsRegular() {
		return result, errors.New("runtime log path is not a regular file")
	}
	result.SizeBytes = info.Size()
	if limit <= 0 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}
	const readLimit = 256 << 10
	start := int64(0)
	if info.Size() > readLimit {
		start = info.Size() - readLimit
	}
	file, err := os.Open(path)
	if err != nil {
		return result, fmt.Errorf("open runtime log: %w", err)
	}
	defer file.Close()
	if start > 0 {
		if _, err := file.Seek(start, io.SeekStart); err != nil {
			return result, fmt.Errorf("seek runtime log: %w", err)
		}
	}
	data, err := io.ReadAll(io.LimitReader(file, readLimit+1))
	if err != nil {
		return result, fmt.Errorf("read runtime log: %w", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
	// The first line is partial when starting in the middle of the file.
	if start > 0 && len(lines) > 0 {
		lines = lines[1:]
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	if len(lines) == 1 && lines[0] == "" {
		lines = []string{}
	}
	result.Lines = lines
	return result, nil
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

	mu         sync.Mutex
	retained   map[string]bool
	retainedAt time.Time
	now        func() time.Time
}

func New(directory string, db *ent.Client, images *mediaimage.Cache, scrapeSvc ArtworkLocker) (*Service, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}
	return &Service{directory: absolute, db: db, images: images, scrape: scrapeSvc, now: time.Now}, nil
}

func (service *Service) Info(ctx context.Context) (Info, error) {
	retained, err := service.cachedRetainedArtwork(ctx)
	if err != nil {
		return Info{}, err
	}
	return service.info(ctx, retained)
}

// cachedRetainedArtwork reuses the last reference snapshot for a short window so
// repeatedly opening the settings page does not rescan the whole movie table.
func (service *Service) cachedRetainedArtwork(ctx context.Context) (map[string]bool, error) {
	service.mu.Lock()
	if service.retained != nil && service.now().Sub(service.retainedAt) < retainedTTL {
		cached := service.retained
		service.mu.Unlock()
		return cached, nil
	}
	service.mu.Unlock()
	retained, err := service.retainedArtwork(ctx)
	if err != nil {
		return nil, err
	}
	service.storeRetained(retained)
	return retained, nil
}

func (service *Service) storeRetained(retained map[string]bool) {
	service.mu.Lock()
	service.retained = retained
	service.retainedAt = service.now()
	service.mu.Unlock()
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
	service.storeRetained(retained)
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
		task.TypeEQ(tasks.KindScrape.String()), task.StatusNEQ(task.StatusDone),
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
