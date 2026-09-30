package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/export"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

// DefaultPacing provides ~2-3 req/s with 150ms jitter for 115 cold-start traversal.
func DefaultPacing(ctx context.Context) error {
	delay := 350*time.Millisecond + time.Duration(rand.N(150))*time.Millisecond
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

// MediaNotifier receives notifications when exported media directories are written or updated.
type MediaNotifier = scrape.MediaNotifier

// Scanner encapsulates the dependencies required to execute library scan jobs.
type Scanner struct {
	driveSvc  *drive.Drive
	db        *ent.Client
	images    *mediaimage.Cache
	tasksSvc  *tasks.Service
	exportMgr *export.Manager
	pace      func(context.Context) error
	notifier  MediaNotifier
}

// New creates a new Scanner with the provided dependencies.
func New(driveSvc *drive.Drive, db *ent.Client, images *mediaimage.Cache, tasksSvc *tasks.Service, exportMgr *export.Manager, pace func(context.Context) error) *Scanner {
	if pace == nil {
		pace = DefaultPacing
	}
	return &Scanner{
		driveSvc:  driveSvc,
		db:        db,
		images:    images,
		tasksSvc:  tasksSvc,
		exportMgr: exportMgr,
		pace:      pace,
	}
}

func (s *Scanner) SetMediaNotifier(notifier MediaNotifier) {
	s.notifier = notifier
}

// Run executes a library scan job: it walks the media directories, matches NFOs
// and videos, indexes movies and files, and enqueues metadata scraping.
func (s *Scanner) Run(ctx context.Context, job tasks.Job) error {
	payload, err := tasks.DecodePayload[domain.ScanPayload](job.Payload)
	if err != nil {
		return err
	}
	if payload.OfflineTaskID != 0 && (payload.TargetID == "" || payload.JavDBID == "" || payload.Code == "") {
		return domain.E(domain.KindInvalid, "离线扫描缺少下载位置或 JavDB 影片信息", nil)
	}
	// Index reconciliation and the next jobs commit together. A restart after
	// that commit only needs to finish this task, not enqueue the jobs again.
	if payload.Scan.Stage == "done" {
		return nil
	}
	sess, err := s.driveSvc.OpenSource(ctx, payload.Source)
	if err != nil {
		return err
	}
	source := sess.Source()
	payload.Source = source
	run := scanRun{scanner: s, session: sess, taskID: job.ID, payload: &payload}
	if payload.Scan.Stage == "reconciling" {
		return run.reconcile(ctx)
	}
	// A fresh scan gets a new marker; a resumed scan reuses its marker so files
	// indexed before an interruption remain valid during reconciliation.
	isResume := payload.ScanID != ""
	if !isResume {
		payload.ScanID = uuid.NewString()
		payload.Checkpoint = ""
		payload.Scan = domain.ScanProgress{
			Stage:                 "scanning",
			CurrentPath:           source.Directory.Path,
			DirectoriesDiscovered: 1,
		}
	} else {
		payload.Scan.Stage = "scanning"
		if payload.Scan.CurrentPath == "" {
			payload.Scan.CurrentPath = source.Directory.Path
		}
	}
	if payload.TargetID != "" {
		return run.runTarget(ctx, isResume)
	}
	return run.walk(ctx, Directory{ID: source.Directory.ID, Path: source.Directory.Path}, isResume)
}

// scanRun holds mutable state for one job; none of it is shared across runs.
type scanRun struct {
	scanner     *Scanner
	session     drive.Session
	taskID      int
	payload     *domain.ScanPayload
	directories []Directory
	seen        map[string]bool
	codes       map[string]bool
	// Persist directory-start counters while its chunks commit independently.
	checkpointProgress *domain.ScanProgress
}

func (r *scanRun) savePage(ctx context.Context, directoryPath string, videos []Video, prepare func([]Video) []Video) error {
	return r.session.Commit(ctx, func(tx *ent.Tx) error {
		return r.processPageTx(ctx, tx, directoryPath, videos, prepare)
	})
}

func (r *scanRun) reconcile(ctx context.Context) error {
	// Cover jobs acquire artwork protection before export configuration and
	// source protection; keep the same order while restoring cached exports.
	if r.scanner.images != nil {
		if err := r.scanner.images.LockArtwork(ctx); err != nil {
			return err
		}
		defer r.scanner.images.UnlockArtwork()
	}
	return r.scanner.exportMgr.WithConfig(func(expCfg export.Config) error {
		finish := func() error {
			reusable, err := r.prepareReconcile(ctx, expCfg)
			if err != nil {
				return err
			}
			return ent.WithTx(ctx, r.scanner.db, func(tx *ent.Tx) error {
				return r.reconcileTx(ctx, tx, expCfg, reusable)
			})
		}
		if r.session != nil {
			return r.session.WithSource(ctx, finish)
		}
		return finish()
	})
}

func (r *scanRun) runTarget(ctx context.Context, isResume bool) error {
	info, err := drive.SourceInfo(ctx, r.session, r.payload.TargetID)
	if err != nil {
		return fmt.Errorf("read completed download: %w", err)
	}
	if r.payload.OfflineTaskID != 0 && info.ID == r.payload.Source.Directory.ID {
		return domain.E(domain.KindConflict, "115 返回的是媒体根目录，无法确定本次下载的影片文件", nil)
	}
	r.payload.TargetPath = drive.FilePath(info.Path, info.Name)
	r.payload.TargetFile = !info.IsDirectory
	if r.payload.TargetFile {
		if !domain.IsVideo(info.Name) {
			return domain.E(domain.KindInvalid, "115 下载结果不是视频文件", nil)
		}
		videos := []Video{IdentifyVideo(info.File)}
		if r.payload.OfflineTaskID != 0 {
			videos[0].Code = r.payload.Code
		}
		if err := resolveTargetNFO(ctx, r.session, videos); err != nil {
			return err
		}
		r.payload.Scan.FilesScanned, r.payload.Scan.VideoFiles = 1, 1
		if err := r.savePage(ctx, path.Dir(r.payload.TargetPath), videos, func(videos []Video) []Video {
			if videos[0].Code != "" {
				r.payload.Scan.MatchedFiles, r.payload.Scan.Movies = 1, 1
			} else {
				r.payload.Scan.UnmatchedFiles = 1
			}
			return videos
		}); err != nil {
			return err
		}
		return r.reconcile(ctx)
	}
	return r.walk(ctx, Directory{ID: info.ID, Path: r.payload.TargetPath}, isResume)
}

func (r *scanRun) walk(ctx context.Context, start Directory, isResume bool) error {
	r.seen = make(map[string]bool)
	if r.payload.Checkpoint != "" {
		_ = json.Unmarshal([]byte(r.payload.Checkpoint), &r.directories)
		for _, d := range r.directories {
			r.seen[d.ID] = true
		}
	}
	if len(r.directories) == 0 {
		r.directories = []Directory{start}
		r.seen[start.ID] = true
	}
	r.codes = make(map[string]bool)
	if isResume {
		if existing, err := r.scanner.db.Movie.Query().Where(movie.HasFilesWith(file.ScanIDEQ(r.payload.ScanID))).Select(movie.FieldCode).Strings(ctx); err == nil {
			for _, code := range existing {
				r.codes[code] = true
			}
		}
	}
	for next := 0; next < len(r.directories); next++ {
		directory := r.directories[next]
		r.payload.Scan.CurrentPath = directory.Path
		data, _ := json.Marshal(r.directories[next:])
		r.payload.Checkpoint = string(data)
		if err := ReportScan(ctx, r.scanner.db.Task, r.taskID, *r.payload, r.scanner.tasksSvc); err != nil {
			return err
		}
		progress := r.payload.Scan
		r.checkpointProgress = &progress
		if err := r.indexDirectory(ctx, directory); err != nil {
			return err
		}
		r.checkpointProgress = nil
	}

	r.payload.Checkpoint = ""
	r.payload.Scan.Stage = "reconciling"
	r.payload.Scan.CurrentPath = r.payload.Source.Directory.Path
	if err := ReportScan(ctx, r.scanner.db.Task, r.taskID, *r.payload, r.scanner.tasksSvc); err != nil {
		return err
	}
	return r.reconcile(ctx)
}

func (r *scanRun) indexDirectory(ctx context.Context, directory Directory) error {
	var directoryVideos []Video
	var sidecars []pan.File

	err := drive.WalkFilePages(ctx, func(offset int) (pan.FilePage, error) {
		if r.scanner.pace != nil {
			if err := r.scanner.pace(ctx); err != nil {
				return pan.FilePage{}, err
			}
		}
		page, err := r.session.List(ctx, directory.ID, offset)
		if err != nil {
			return pan.FilePage{}, err
		}
		if !slices.ContainsFunc(page.Path, func(d pan.Directory) bool { return d.ID == r.payload.Source.Directory.ID }) {
			return pan.FilePage{}, domain.E(domain.KindConflict, "该文件夹已移出媒体目录，请重新扫描", nil)
		}
		return page, nil
	}, func(page pan.FilePage) (bool, error) {
		for _, entry := range page.Files {
			if r.seen[entry.ID] {
				continue
			}
			r.seen[entry.ID] = true
			if entry.IsDirectory {
				r.directories = append(r.directories, Directory{ID: entry.ID, Path: path.Join(directory.Path, entry.Name)})
				r.payload.Scan.DirectoriesDiscovered++
				continue
			}
			r.payload.Scan.FilesScanned++
			if strings.EqualFold(path.Ext(entry.Name), ".nfo") {
				sidecars = append(sidecars, entry)
			}
			if !domain.IsVideo(entry.Name) {
				continue
			}
			r.payload.Scan.VideoFiles++
			directoryVideos = append(directoryVideos, IdentifyVideo(entry))
		}
		if !page.HasMore {
			r.payload.Scan.DirectoriesScanned++
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("scan %s: %w", directory.Path, err)
	}

	// Apply single-NFO tolerance matching to establish standard catalogue identity.
	if err := ResolveNFOCodes(ctx, r.session, sidecars, directoryVideos); err != nil {
		return err
	}

	// Save directory videos in chunks of 100.
	for startIdx := 0; startIdx < len(directoryVideos); startIdx += 100 {
		chunk := directoryVideos[startIdx:min(startIdx+100, len(directoryVideos))]
		if err := r.savePage(ctx, directory.Path, chunk, func(identified []Video) []Video {
			for _, video := range identified {
				if video.Code != "" {
					r.payload.Scan.MatchedFiles++
					r.codes[video.Code] = true
				} else {
					r.payload.Scan.UnmatchedFiles++
				}
			}

			r.payload.Scan.Movies = len(r.codes)
			return identified
		}); err != nil {
			return err
		}
	}
	return nil
}
