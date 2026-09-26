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
	embyDir   string
	publicURL string
	strmToken string
	pace      func(context.Context) error
	notifier  MediaNotifier
}

// New creates a new Scanner with the provided dependencies.
func New(driveSvc *drive.Drive, db *ent.Client, images *mediaimage.Cache, tasksSvc *tasks.Service) *Scanner {
	return &Scanner{
		driveSvc: driveSvc,
		db:       db,
		images:   images,
		tasksSvc: tasksSvc,
	}
}

func (s *Scanner) SetEmbyExport(embyDir, publicURL, strmToken string) {
	s.embyDir = embyDir
	s.publicURL = publicURL
	s.strmToken = strmToken
}

func (s *Scanner) SetPacing(pace func(context.Context) error) {
	s.pace = pace
}

func (s *Scanner) SetMediaNotifier(notifier MediaNotifier) {
	s.notifier = notifier
}

// Run executes a library scan job: it walks the media directories, matches NFOs
// and videos, indexes movies and files, and enqueues metadata scraping.
func (s *Scanner) Run(ctx context.Context, job tasks.Job) error {
	driveSvc, db, images, tasksSvc := s.driveSvc, s.db, s.images, s.tasksSvc

	payload, err := tasks.DecodePayload[Payload](job.Payload)
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
	sess, err := driveSvc.OpenSource(ctx, payload.Source)
	if err != nil {
		return err
	}
	source := sess.Source()
	// Each execution gets a fresh marker, including after a server restart. An
	// interrupted attempt must not make unvisited files look present on retry.
	scanID := uuid.NewString()
	payload.Source = source
	payload.Scan = domain.ScanProgress{
		Stage:                 "scanning",
		CurrentPath:           source.Directory.Path,
		DirectoriesDiscovered: 1,
	}
	start := Directory{ID: source.Directory.ID, Path: source.Directory.Path}
	observed := make(scrape.DirectoryObservations)

	savePage := func(directoryPath string, videos []Video, prepare func([]Video) []Video) error {
		return sess.Commit(ctx, func(tx *ent.Tx) error {
			return ProcessScanPageTx(ctx, tx, job.ID, scanID, directoryPath, videos, &payload, prepare, tasksSvc)
		})
	}
	reconcile := func() error {
		return sess.Commit(ctx, func(tx *ent.Tx) error {
			return ReconcileScanTx(ctx, tx, job.ID, scanID, &payload, observed, images, tasksSvc, s.embyDir, s.publicURL, s.strmToken, s.notifier)
		})
	}

	if payload.TargetID != "" {
		info, err := drive.SourceInfo(ctx, sess, payload.TargetID)
		if err != nil {
			return fmt.Errorf("read completed download: %w", err)
		}
		if payload.OfflineTaskID != 0 && info.ID == source.Directory.ID {
			return domain.E(domain.KindConflict, "115 返回的是媒体根目录，无法确定本次下载的影片文件", nil)
		}
		payload.TargetPath = drive.FilePath(info.Path, info.Name)
		payload.TargetFile = !info.IsDirectory
		if payload.TargetFile {
			if !domain.IsVideo(info.Name) {
				return domain.E(domain.KindInvalid, "115 下载结果不是视频文件", nil)
			}
			payload.Scan.FilesScanned, payload.Scan.VideoFiles = 1, 1
			if err := savePage(path.Dir(payload.TargetPath), []Video{{File: info.File}}, func(videos []Video) []Video {
				if videos[0].Code != "" {
					payload.Scan.MatchedFiles, payload.Scan.Movies = 1, 1
				} else {
					payload.Scan.UnmatchedFiles = 1
				}
				return videos
			}); err != nil {
				return err
			}
			entries, err := drive.DirectoryEntries(ctx, sess, info.ParentID)
			if err != nil {
				return err
			}
			observed.Add(info.ParentID, entries)
			return reconcile()
		}
		start = Directory{ID: info.ID, Path: payload.TargetPath}
	}

	var directories []Directory
	seen := make(map[string]bool)
	if payload.Checkpoint != "" {
		_ = json.Unmarshal([]byte(payload.Checkpoint), &directories)
		for _, d := range directories {
			seen[d.ID] = true
		}
	}
	if len(directories) == 0 {
		directories = []Directory{start}
		seen[start.ID] = true
	}
	codes := make(map[string]bool)
	lastReport := time.Time{}

	for next := 0; next < len(directories); next++ {
		directory := directories[next]
		payload.Scan.CurrentPath = directory.Path
		if time.Since(lastReport) >= 500*time.Millisecond || next == len(directories)-1 {
			if remaining := directories[next:]; len(remaining) > 0 {
				data, _ := json.Marshal(remaining)
				payload.Checkpoint = string(data)
			}
			if err := ReportScan(ctx, db.Task, job.ID, payload, tasksSvc); err != nil {
				return err
			}
			lastReport = time.Now()
		}
		var directoryVideos []Video
		var sidecars []pan.File

		err := drive.WalkFilePages(ctx, func(offset int) (pan.FilePage, error) {
			if s.pace != nil {
				if err := s.pace(ctx); err != nil {
					return pan.FilePage{}, err
				}
			}
			page, err := sess.List(ctx, directory.ID, offset)
			if err != nil {
				return pan.FilePage{}, err
			}
			if !slices.ContainsFunc(page.Path, func(d pan.Directory) bool { return d.ID == source.Directory.ID }) {
				return pan.FilePage{}, domain.E(domain.KindConflict, "该文件夹已移出媒体目录，请重新扫描", nil)
			}
			return page, nil
		}, func(page pan.FilePage) (bool, error) {
			observed.Add(directory.ID, page.Files)
			for _, entry := range page.Files {
				if seen[entry.ID] {
					continue
				}
				seen[entry.ID] = true
				if entry.IsDirectory {
					directories = append(directories, Directory{ID: entry.ID, Path: path.Join(directory.Path, entry.Name)})
					payload.Scan.DirectoriesDiscovered++
					continue
				}
				payload.Scan.FilesScanned++
				if strings.EqualFold(path.Ext(entry.Name), ".nfo") {
					sidecars = append(sidecars, entry)
				}
				if !domain.IsVideo(entry.Name) {
					continue
				}
				payload.Scan.VideoFiles++
				directoryVideos = append(directoryVideos, IdentifyVideo(entry))
			}
			if !page.HasMore {
				payload.Scan.DirectoriesScanned++
			}
			return true, nil
		})
		if err != nil {
			return fmt.Errorf("scan %s: %w", directory.Path, err)
		}

		// Apply single-NFO tolerance matching to establish standard catalogue identity.
		if err := ResolveSingleNFO(ctx, sess, sidecars, directoryVideos); err != nil {
			return err
		}

		// Save directory videos in chunks of 100.
		for startIdx := 0; startIdx < len(directoryVideos); startIdx += 100 {
			chunk := directoryVideos[startIdx:min(startIdx+100, len(directoryVideos))]
			if err := savePage(directory.Path, chunk, func(identified []Video) []Video {
				for _, video := range identified {
					if video.Code != "" {
						payload.Scan.MatchedFiles++
						codes[video.Code] = true
					} else {
						payload.Scan.UnmatchedFiles++
					}
				}

				payload.Scan.Movies = len(codes)
				return identified
			}); err != nil {
				return err
			}
		}
	}

	payload.Checkpoint = ""
	payload.Scan.Stage = "reconciling"
	payload.Scan.CurrentPath = source.Directory.Path
	if err := ReportScan(ctx, db.Task, job.ID, payload, tasksSvc); err != nil {
		return err
	}
	return reconcile()
}
