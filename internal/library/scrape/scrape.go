package scrape

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	subtitlemeta "github.com/ppxb/miyabi/internal/domain/subtitle"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/export"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

// MetadataPayload describes the input for a movie scrape job.
type MetadataPayload struct {
	Rebuild    bool                 `json:"rebuild,omitempty"`
	Source     domain.LibrarySource `json:"source"`
	ScanTaskID int                  `json:"scan_task_id"`
	MovieID    int                  `json:"movie_id"`
	Code       string               `json:"code"`
	JavDBID    string               `json:"javdb_id,omitempty"`
	ManualCode string               `json:"manual_code,omitempty"`
}

// Payload holds the checkpoints of one movie scraping workflow.
type Payload struct {
	MetadataPayload
	MetadataReady bool                `json:"metadata_ready,omitempty"`
	Document      nfo.Movie           `json:"document"`
	Artwork       *mediaimage.Artwork `json:"artwork,omitempty"`
	PosterVersion int                 `json:"poster_version,omitempty"`
	Completed     bool                `json:"completed,omitempty"`
}

// MetadataSource resolves independent sources and downloads their image candidates.
type MetadataSource interface {
	Resolve(context.Context, domain.MovieRef) (domain.MovieMetadata, error)
	Fallback(context.Context, domain.MovieRef) (domain.MovieMetadata, error)
	Image(context.Context, domain.ImageCandidate) (domain.Media, error)
}

// Notifier reports committed library updates and queued follow-up work.
type Notifier interface {
	NotifyLibraryChanged()
	WakePool()
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
	metadata      MetadataSource
	images        *mediaimage.Cache
	notifier      Notifier
	subtitles     SubtitleExporter
	subtitleQueue *SubtitleQueue

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
func New(db *ent.Client, d *drive.Drive, metadata MetadataSource, images *mediaimage.Cache, notifier Notifier, deps Dependencies) *Service {
	service := &Service{
		db:            db,
		drive:         d,
		metadata:      metadata,
		images:        images,
		notifier:      notifier,
		dirCache:      make(map[string]dirCacheEntry),
		exportMgr:     deps.ExportManager,
		mediaNotifier: deps.MediaNotifier,
		subtitles:     deps.Subtitles,
	}
	service.subtitleQueue = newSubtitleQueue(service)
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
	service.subtitleQueue.Close()
}

// TryLockArtwork attempts to acquire the artwork lock for cache maintenance.
func (service *Service) TryLockArtwork() bool {
	return service.images.TryLockArtwork()
}

// UnlockArtwork releases the artwork lock.
func (service *Service) UnlockArtwork() {
	service.images.UnlockArtwork()
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

// Scrape runs one recoverable movie workflow: metadata, artwork, then export.
func (service *Service) Scrape(ctx context.Context, job tasks.Job) error {
	input, err := tasks.DecodePayload[Payload](job.Payload)
	if err != nil {
		return err
	}
	if input.Completed {
		return nil
	}
	input.Code = codeid.Normalize(input.Code)
	sess, err := service.begin(ctx, input.MetadataPayload)
	if err != nil {
		return err
	}
	current, err := service.db.Movie.Query().Where(movie.IDEQ(input.MovieID)).Select(movie.FieldID, movie.FieldManualCode).Only(ctx)
	if err != nil {
		return err
	}
	if current.ManualCode != input.ManualCode {
		return domain.E(domain.KindConflict, "影片番号已纠正，请使用新的刮削任务", nil)
	}
	if !input.MetadataReady {
		if err := service.prepareMetadata(ctx, sess, job.ID, &input); err != nil {
			return err
		}
	}
	if err := tasks.Checkpoint(ctx, service.db); err != nil {
		return err
	}
	if err := service.prepareArtwork(ctx, job.ID, &input); err != nil {
		return err
	}
	if err := tasks.Checkpoint(ctx, service.db); err != nil {
		return err
	}
	subTask, err := service.publishMovie(ctx, sess, job, input)
	if err != nil {
		return err
	}
	if subTask != nil {
		service.subtitleQueue.Enqueue(*subTask)
	}
	return nil
}

func (service *Service) prepareMetadata(ctx context.Context, sess drive.Session, taskID int, input *Payload) error {
	record, err := service.db.Movie.Query().Where(movie.IDEQ(input.MovieID),
		movie.HasFilesWith(database.LibraryFiles(input.Source))).Only(ctx)
	if err != nil {
		return fmt.Errorf("load indexed movie for metadata: %w", err)
	}
	if record.ScrapeStatus == movie.ScrapeStatusDone && !input.Rebuild {
		input.Document, err = MovieNFO(record)
		if err != nil {
			return err
		}
		artwork := MovieArtwork(record)
		cached, err := service.images.Exists(artwork)
		if err != nil {
			return err
		}
		if cached {
			input.Artwork = &artwork
		}
		if record.MetadataSnapshot != nil {
			input.PosterVersion = record.MetadataSnapshot.PosterVersion
		}
	} else {
		if id := domain.ValueOrZero(record.JavdbID); id != "" {
			input.JavDBID = id
		}
		if err := service.resolveMetadata(ctx, input); err != nil {
			return err
		}
	}
	input.Code = codeid.Normalize(input.Document.Code)
	input.Document.Code = input.Code
	input.MetadataReady = true
	encoded, err := tasks.EncodePayload(input)
	if err != nil {
		return err
	}
	if err := sess.Commit(ctx, func(tx *ent.Tx) error {
		if err := SaveMovieMetadata(ctx, tx, input.MovieID, input.Document); err != nil {
			return err
		}
		return tx.Task.UpdateOneID(taskID).SetPayload(encoded).Exec(ctx)
	}); err != nil {
		return fmt.Errorf("save movie metadata checkpoint: %w", err)
	}
	if service.notifier != nil {
		service.notifier.NotifyLibraryChanged()
	}
	return nil
}

// Finished marks the movie scrape as failed inside the completion transaction
// when a scrape job fails. Failures change the library view; successes
// only advance the download workflow projection.
func (service *Service) Finished(ctx context.Context, tx *ent.Tx, job tasks.Job, result error) (tasks.Change, error) {
	if result == nil {
		return tasks.ChangeOffline, nil
	}
	input, err := tasks.DecodePayload[MetadataPayload](job.Payload)
	if err != nil {
		return 0, err
	}
	update := tx.Movie.Update().Where(
		movie.IDEQ(input.MovieID),
		movie.ManualCodeEQ(input.ManualCode),
		movie.HasFilesWith(database.LibraryFiles(input.Source)),
	)
	if !input.Rebuild {
		update.Where(movie.ScrapeStatusNEQ(movie.ScrapeStatusDone))
	}
	if err := update.SetScrapeStatus(movie.ScrapeStatusFailed).Exec(ctx); err != nil {
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
	service.dirCache[key] = dirCacheEntry{
		files:     files,
		expiresAt: time.Now().Add(defaultDirCacheTTL),
	}
	service.dirMu.Unlock()

	return slices.Clone(files), nil
}

func (service *Service) resolveMetadata(ctx context.Context, input *Payload) error {
	result, err := service.metadata.Resolve(ctx, domain.MovieRef{Code: input.Code, JavDBID: input.JavDBID, Refresh: input.Rebuild})
	if err != nil {
		return err
	}
	input.Document = DetailNFO(result.Detail)
	input.Document.Images = result.Images
	input.Code = codeid.Normalize(input.Document.Code)
	input.Document.Code = input.Code
	return nil
}
