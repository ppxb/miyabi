package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/library/scan"
	"github.com/ppxb/miyabi/internal/tasks"
)

const embyLocalDirectoryName = "Emby 本地目录"

// localMetadataJob mirrors scrape.MetadataPayload so the library package need
// not import the scrape package just to queue metadata work.
type localMetadataJob struct {
	Rebuild    bool                 `json:"rebuild,omitempty"`
	Source     domain.LibrarySource `json:"source"`
	ScanTaskID int                  `json:"scan_task_id"`
	MovieID    int                  `json:"movie_id"`
	Code       string               `json:"code"`
	ManualCode string               `json:"manual_code,omitempty"`
}

// startLocalScan queues an import of the configured Emby directory.
func (s *Service) startLocalScan(ctx context.Context) (domain.TaskInfo, error) {
	root, err := s.localScanRoot()
	if err != nil {
		return domain.TaskInfo{}, err
	}
	return s.EnqueueScanMode(ctx, domain.LibrarySource{
		AccountID: domain.LocalAccountID,
		Directory: domain.LibraryDirectory{ID: root, Name: embyLocalDirectoryName, Path: root},
	}, true)
}

// ScheduleLocalScan also runs at startup, where a fresh install may not have
// created its export directory yet. No media needs importing in that case.
func (s *Service) ScheduleLocalScan(ctx context.Context) error {
	if s.exportMgr.Config().EmbyDir == "" {
		return nil
	}
	_, err := s.startLocalScan(ctx)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// StartLocalSourceScan queues an import of any user-configured local directory,
// keeping only local metadata by default (Emby exports already carry sidecars).
func (s *Service) StartLocalSourceScan(ctx context.Context, root string) (domain.TaskInfo, error) {
	return s.StartLocalSourceScanMode(ctx, root, true)
}

// StartLocalSourceScanMode queues a local scan that either auto-completes
// metadata or reads only local data.
func (s *Service) StartLocalSourceScanMode(ctx context.Context, root string, metadataOnly bool) (domain.TaskInfo, error) {
	return s.startLocalSourceScan(ctx, root, metadataOnly, false)
}

// StartLocalSourceScanModeIncremental only completes movies that still lack metadata.
func (s *Service) StartLocalSourceScanModeIncremental(ctx context.Context, root string, metadataOnly bool) (domain.TaskInfo, error) {
	return s.startLocalSourceScan(ctx, root, metadataOnly, true)
}

func (s *Service) startLocalSourceScan(ctx context.Context, root string, metadataOnly, incremental bool) (domain.TaskInfo, error) {
	canonical, err := canonicalDirectory(root)
	if err != nil {
		return domain.TaskInfo{}, err
	}
	source := domain.LibrarySource{
		AccountID: domain.LocalAccountID,
		Directory: domain.LibraryDirectory{ID: canonical, Name: "本地目录", Path: canonical},
	}
	if incremental {
		return s.EnqueueSyncMode(ctx, source, metadataOnly)
	}
	return s.EnqueueScanMode(ctx, source, metadataOnly)
}

func (s *Service) localScanRoot() (string, error) {
	dir := s.exportMgr.Config().EmbyDir
	if dir == "" {
		return "", domain.E(domain.KindInvalid, "未配置 Emby 本地目录", nil)
	}
	return canonicalDirectory(dir)
}

func canonicalDirectory(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}

func (s *Service) scanLocal(ctx context.Context, id int, payload domain.ScanPayload) error {
	root := payload.Source.Directory.ID
	if payload.Source.Directory.Name == embyLocalDirectoryName {
		current, err := s.localScanRoot()
		if err != nil {
			return err
		}
		if current != root {
			return domain.E(domain.KindConflict, "Emby 本地目录已变更，请扫描当前目录", nil)
		}
	}
	if root == "" {
		return domain.E(domain.KindInvalid, "本地扫描缺少目录", nil)
	}
	payload.Scan = domain.ScanProgress{Stage: "scanning", CurrentPath: root}
	if err := scan.ReportScan(ctx, s.database.Task, id, payload, s.tasks); err != nil {
		return err
	}
	result, err := s.localScanner.Scan(ctx, root)
	if err != nil {
		return fmt.Errorf("scan local directory: %w", err)
	}
	if err := s.applyLocalMetadataMode(ctx, id, payload, result.PendingMovieIDs); err != nil {
		return err
	}
	payload.Scan.Stage = "done"
	payload.Scan.FilesScanned = result.FilesScanned
	payload.Scan.VideoFiles = result.MediaFiles
	payload.Scan.MatchedFiles = result.MediaFiles
	payload.Scan.Movies = result.MoviesAdded
	return scan.ReportScan(ctx, s.database.Task, id, payload, s.tasks)
}

// applyLocalMetadataMode either settles local-only movies or queues metadata
// completion for the movies a local scan discovered without local metadata.
func (s *Service) applyLocalMetadataMode(ctx context.Context, scanTaskID int, payload domain.ScanPayload, movieIDs []int) error {
	if len(movieIDs) == 0 {
		return nil
	}
	if payload.MetadataOnly {
		_, err := s.database.Movie.Update().
			Where(movie.IDIn(movieIDs...), movie.ScrapeStatusEQ(movie.ScrapeStatusPending)).
			SetScrapeStatus(movie.ScrapeStatusDone).Save(ctx)
		return err
	}
	records, err := s.database.Movie.Query().
		Where(movie.IDIn(movieIDs...)).
		Select(movie.FieldID, movie.FieldCode, movie.FieldManualCode).
		All(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		input := localMetadataJob{
			Source: payload.Source, ScanTaskID: scanTaskID, MovieID: record.ID,
			Code: record.Code, ManualCode: record.ManualCode,
		}
		encoded, err := tasks.EncodePayload(input)
		if err != nil {
			return err
		}
		resourceKey := fmt.Sprintf("movie:%d", record.ID)
		exists, err := s.database.Task.Query().Where(
			task.TypeEQ(tasks.KindScrape.String()), task.ResourceKeyEQ(resourceKey),
			task.StatusIn(task.StatusQueued, task.StatusRunning),
		).Exist(ctx)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := s.database.Task.Create().SetType(tasks.KindScrape.String()).
			SetResourceKey(resourceKey).SetPayload(encoded).Save(ctx); err != nil {
			return err
		}
	}
	s.tasks.WakePool()
	return nil
}
