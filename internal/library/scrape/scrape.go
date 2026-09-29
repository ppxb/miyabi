package scrape

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	subtitlemeta "github.com/ppxb/miyabi/internal/domain/subtitle"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/export"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/syncx"
	"github.com/ppxb/miyabi/internal/tasks"
)

// MetadataPayload describes the input for a movie scrape job.
type MetadataPayload struct {
	Source     domain.LibrarySource `json:"source"`
	ScanTaskID int                  `json:"scan_task_id"`
	MovieID    int                  `json:"movie_id"`
	Code       string               `json:"code"`
	JavDBID    string               `json:"javdb_id,omitempty"`
}

// Discoverer abstracts catalogue queries and media downloads.
type Discoverer interface {
	ResolveMovieID(ctx context.Context, code string) (string, error)
	CatalogueDetail(ctx context.Context, id string) (domain.MovieDetail, error)
	Media(ctx context.Context, url string) (domain.Media, error)
}

// Notifier notifies subscribers that library contents changed.
type Notifier interface {
	NotifyLibraryChanged()
}

// SubtitleExporter writes a movie's subtitles beside its exported .strm file.
type SubtitleExporter interface {
	Export(ctx context.Context, movieID int, target subtitlemeta.Target) (int, error)
}

const defaultDirCacheTTL = 45 * time.Second

type dirCacheEntry struct {
	files     []pan.File
	expiresAt time.Time
}

// Service manages movie metadata scraping and artwork caching.
type Service struct {
	db            *ent.Client
	drive         *drive.Drive
	discover      Discoverer
	images        *mediaimage.Cache
	notifier      Notifier
	subtitles     SubtitleExporter
	subtitleQueue *SubtitleQueue
	artwork       syncx.ContextLock

	dirMu    sync.RWMutex
	dirCache map[string]dirCacheEntry

	exportMgr     *export.Manager
	mediaNotifier MediaNotifier
}

func (service *Service) exportConfig() export.Config {
	if service.exportMgr != nil {
		return service.exportMgr.Config()
	}
	return export.Config{}
}

type Dependencies struct {
	ExportManager *export.Manager
	MediaNotifier MediaNotifier
	Subtitles     SubtitleExporter
}

// New installs dependencies before starting subtitle workers.
func New(db *ent.Client, d *drive.Drive, discover Discoverer, images *mediaimage.Cache, notifier Notifier, deps Dependencies) *Service {
	service := &Service{
		db:            db,
		drive:         d,
		discover:      discover,
		images:        images,
		notifier:      notifier,
		dirCache:      make(map[string]dirCacheEntry),
		exportMgr:     deps.ExportManager,
		mediaNotifier: deps.MediaNotifier,
		subtitles:     deps.Subtitles,
	}
	service.subtitleQueue = newSubtitleQueue(service, defaultSubtitleConcurrency, defaultSubtitleQueueCapacity, nil)
	return service
}

// SetEmbyExport configures the local Emby export directory, public URL written into .strm files, and optional STRM token.
func (service *Service) SetEmbyExport(embyDir, publicURL, strmToken string) {
	if service.exportMgr == nil {
		service.exportMgr = export.NewManager(export.Config{})
	}
	service.exportMgr.Set(export.Config{
		EmbyDir:   embyDir,
		PublicURL: publicURL,
		STRMToken: strmToken,
	})
}

// Close releases resources and terminates background workers.
func (service *Service) Close() {
	if service == nil {
		return
	}
	if service.subtitleQueue != nil {
		service.subtitleQueue.Close()
	}
}

// TryLockArtwork attempts to acquire the artwork lock for cache maintenance.
func (service *Service) TryLockArtwork() bool {
	return service.artwork.TryLock()
}

// UnlockArtwork releases the artwork lock.
func (service *Service) UnlockArtwork() {
	service.artwork.Unlock()
}

// Artwork reads cached artwork bytes by key.
func (service *Service) Artwork(key string) ([]byte, error) {
	return service.images.Read(key)
}

func (service *Service) begin(ctx context.Context, input MetadataPayload) (drive.Session, error) {
	if service.drive == nil {
		return nil, domain.E(domain.KindInvalid, "网盘服务未初始化", nil)
	}
	return service.drive.OpenSource(ctx, input.Source)
}

// Scrape processes a movie scrape task.
func (service *Service) Scrape(ctx context.Context, job tasks.Job) error {
	input, err := tasks.DecodePayload[MetadataPayload](job.Payload)
	if err != nil {
		return err
	}
	input.Code = codeid.Normalize(input.Code)
	// A committed cover job means the metadata transaction already succeeded.
	queued, err := service.db.Task.Query().Where(task.TypeEQ(tasks.KindCover.String()), func(s *sql.Selector) {
		s.Where(sqljson.ValueEQ(task.FieldPayload, job.ID, sqljson.Path("scrape_task_id")))
	}).Exist(ctx)
	if err != nil {
		return fmt.Errorf("find queued artwork: %w", err)
	}
	if queued {
		return nil
	}
	sess, err := service.begin(ctx, input)
	if err != nil {
		return err
	}
	record, err := service.db.Movie.Query().Where(movie.IDEQ(input.MovieID),
		movie.HasFilesWith(database.LibraryFiles(input.Source))).WithActors().WithTags().Only(ctx)
	if err != nil {
		return fmt.Errorf("load indexed movie for metadata: %w", err)
	}
	cover := CoverPayload{MetadataPayload: input, ScrapeTaskID: job.ID}
	if record.ScrapeStatus == movie.ScrapeStatusDone {
		cover.Document = MovieNFO(record)
		artwork := MovieArtwork(record)
		cover.Artwork = &artwork
	} else {
		if id := domain.ValueOrZero(record.JavdbID); id != "" {
			cover.JavDBID = id
		}
		if err := service.loadCatalogueCover(ctx, &cover); err != nil {
			return err
		}
	}
	cover.Code = codeid.Normalize(cover.Document.Code)
	cover.Document.Code = cover.Code
	encoded, err := tasks.EncodePayload(cover)
	if err != nil {
		return err
	}
	if err := sess.Commit(ctx, func(tx *ent.Tx) error {
		if err := SaveMovieMetadata(ctx, tx, input.MovieID, cover.Document); err != nil {
			return err
		}
		return tx.Task.Create().SetType(tasks.KindCover.String()).SetPayload(encoded).Exec(ctx)
	}); err != nil {
		return fmt.Errorf("save movie metadata and queue artwork: %w", err)
	}
	if service.notifier != nil {
		service.notifier.NotifyLibraryChanged()
	}
	return nil
}

// Finished marks the movie scrape as failed inside the completion transaction
// when a scrape or cover job fails. Failures change the library view; successes
// only advance the download workflow projection.
func (service *Service) Finished(ctx context.Context, tx *ent.Tx, job tasks.Job, result error) (tasks.Change, error) {
	if result == nil {
		return tasks.ChangeOffline, nil
	}
	input, err := tasks.DecodePayload[MetadataPayload](job.Payload)
	if err != nil {
		return 0, err
	}
	if err := tx.Movie.Update().Where(
		movie.IDEQ(input.MovieID),
		movie.ScrapeStatusNEQ(movie.ScrapeStatusDone),
		movie.HasFilesWith(database.LibraryFiles(input.Source)),
	).SetScrapeStatus(movie.ScrapeStatusFailed).Exec(ctx); err != nil {
		return 0, err
	}
	return tasks.ChangeLibrary, nil
}

// MovieDirectory groups a movie's indexed videos with the current directory listing.
type MovieDirectory struct {
	ID       string
	VideoIDs map[string]bool
	Files    []pan.File
}

// Directories returns all directories containing media files for the movie.
func (service *Service) Directories(ctx context.Context, sess drive.Session, input MetadataPayload) ([]MovieDirectory, error) {
	files, err := service.db.File.Query().Where(database.LibraryFiles(input.Source), file.MovieIDEQ(input.MovieID)).
		Order(ent.Asc(file.FieldParentID), ent.Asc(file.FieldFileID)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load movie file directories: %w", err)
	}
	if len(files) == 0 {
		return nil, domain.E(domain.KindNotFound, "影片已没有媒体文件，请重新扫描", nil)
	}
	var result []MovieDirectory
	byID := make(map[string]int)
	for _, entry := range files {
		index, found := byID[entry.ParentID]
		if !found {
			index = len(result)
			byID[entry.ParentID] = index
			result = append(result, MovieDirectory{ID: entry.ParentID, VideoIDs: make(map[string]bool)})
		}
		result[index].VideoIDs[entry.FileID] = true
	}
	for i := range result {
		directory := &result[i]
		directory.Files, err = service.directoryEntries(ctx, sess, directory.ID)
		if err != nil {
			return nil, fmt.Errorf("read metadata directory: %w", err)
		}
		present := 0
		for _, entry := range directory.Files {
			if !entry.IsDirectory && domain.IsVideo(entry.Name) && entry.Size >= domain.MinVideoSize {
				if directory.VideoIDs[entry.ID] {
					present++
				}
			}
		}
		if present == 0 {
			return nil, domain.E(domain.KindNotFound, "视频文件已删除或移动，请重新扫描", nil)
		}
	}
	return result, nil
}

// directoryEntries retrieves directory entries using a short-lived cache to avoid redundant pagination of large folders.
func (service *Service) directoryEntries(ctx context.Context, sess drive.Session, dirID string) ([]pan.File, error) {
	key := dirID
	if src := sess.Source(); src.AccountID != "" {
		key = src.AccountID + ":" + dirID
	}

	service.dirMu.RLock()
	if entry, ok := service.dirCache[key]; ok && time.Now().Before(entry.expiresAt) {
		service.dirMu.RUnlock()
		return slices.Clone(entry.files), nil
	}
	service.dirMu.RUnlock()

	files, err := drive.DirectoryEntries(ctx, sess, dirID)
	if err != nil {
		return nil, err
	}

	service.dirMu.Lock()
	if service.dirCache == nil {
		service.dirCache = make(map[string]dirCacheEntry)
	}
	service.dirCache[key] = dirCacheEntry{
		files:     files,
		expiresAt: time.Now().Add(defaultDirCacheTTL),
	}
	service.dirMu.Unlock()

	return slices.Clone(files), nil
}
