package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/library/scan"
	"github.com/ppxb/miyabi/internal/localsource"
)

// startLocalScan queues an import of the configured Emby directory.
func (s *Service) startLocalScan(ctx context.Context) (domain.TaskInfo, error) {
	root, err := s.localScanRoot()
	if err != nil {
		return domain.TaskInfo{}, err
	}
	return s.EnqueueScan(ctx, domain.LibrarySource{
		AccountID: domain.LocalAccountID,
		Directory: domain.LibraryDirectory{ID: root, Name: "Emby 本地目录", Path: root},
	})
}

// StartLocalScan queues a scan of one configured local media directory.
func (s *Service) StartLocalScan(ctx context.Context, id string) (domain.TaskInfo, error) {
	if s.localSources == nil {
		return domain.TaskInfo{}, domain.E(domain.KindNotFound, "本地媒体目录不存在", nil)
	}
	source, found, err := s.localSources.Find(ctx, id)
	if err != nil {
		return domain.TaskInfo{}, err
	}
	if !found || !source.Enabled {
		return domain.TaskInfo{}, domain.E(domain.KindNotFound, "本地媒体目录不存在或已停用", nil)
	}
	return s.EnqueueScan(ctx, localLibrarySource(source))
}

// localLibrarySource identifies a configured directory in scan payloads, so
// queued scans, task listings and per-source rescans share one identity.
func localLibrarySource(source localsource.Source) domain.LibrarySource {
	return domain.LibrarySource{
		AccountID: domain.LocalAccountID,
		Directory: domain.LibraryDirectory{ID: source.ID, Name: source.Name, Path: source.Path},
	}
}

// ScheduleLocalScan also runs at startup, where a fresh install may not have
// created its export directory yet. No media needs importing in that case.
func (s *Service) ScheduleLocalScan(ctx context.Context) error {
	_, err := s.startLocalScan(ctx)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *Service) localScanRoot() (string, error) {
	dir := s.exportMgr.Config().EmbyDir
	if dir == "" {
		return "", domain.E(domain.KindInvalid, "未配置 Emby 本地目录", nil)
	}
	return canonicalDirectory(dir)
}

// resolveLocalDirectory returns the directory a queued local scan must read. A
// scan whose source was removed or reconfigured must not read the old path.
func (s *Service) resolveLocalDirectory(ctx context.Context, queued domain.LibrarySource) (string, error) {
	if root, err := s.localScanRoot(); err == nil && root == queued.Directory.ID {
		return root, nil
	}
	stale := domain.E(domain.KindConflict, "本地媒体目录已变更，请重新扫描", nil)
	if s.localSources == nil {
		return "", stale
	}
	source, found, err := s.localSources.Find(ctx, queued.Directory.ID)
	if err != nil {
		return "", err
	}
	if !found || !source.Enabled || source.Path != queued.Directory.Path {
		return "", stale
	}
	return source.Path, nil
}

func canonicalDirectory(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}

func (s *Service) scanLocal(ctx context.Context, id int, payload domain.ScanPayload) error {
	root, err := s.resolveLocalDirectory(ctx, payload.Source)
	if err != nil {
		return err
	}
	payload.Scan = domain.ScanProgress{Stage: "scanning", CurrentPath: root}
	if err := scan.ReportScan(ctx, s.database.Task, id, payload, s.tasks); err != nil {
		return err
	}
	result, err := s.localScanner.Scan(ctx, root)
	if err != nil {
		return fmt.Errorf("scan local directory %s: %w", root, err)
	}
	payload.Scan.Stage = "done"
	payload.Scan.FilesScanned = result.FilesScanned
	payload.Scan.VideoFiles = result.MediaFiles
	payload.Scan.MatchedFiles = result.MediaFiles
	payload.Scan.Movies = result.MoviesAdded
	return scan.ReportScan(ctx, s.database.Task, id, payload, s.tasks)
}
