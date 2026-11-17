package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/library/scan"
)

const embyLocalDirectoryName = "Emby 本地目录"

// startLocalScan queues an import of the configured Emby directory.
func (s *Service) startLocalScan(ctx context.Context) (domain.TaskInfo, error) {
	root, err := s.localScanRoot()
	if err != nil {
		return domain.TaskInfo{}, err
	}
	return s.EnqueueScan(ctx, domain.LibrarySource{
		AccountID: domain.LocalAccountID,
		Directory: domain.LibraryDirectory{ID: root, Name: embyLocalDirectoryName, Path: root},
	})
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

// StartLocalSourceScan queues an import of any user-configured local directory.
func (s *Service) StartLocalSourceScan(ctx context.Context, root string) (domain.TaskInfo, error) {
	canonical, err := canonicalDirectory(root)
	if err != nil {
		return domain.TaskInfo{}, err
	}
	return s.EnqueueScan(ctx, domain.LibrarySource{
		AccountID: domain.LocalAccountID,
		Directory: domain.LibraryDirectory{ID: canonical, Name: "本地目录", Path: canonical},
	})
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
	payload.Scan.Stage = "done"
	payload.Scan.FilesScanned = result.FilesScanned
	payload.Scan.VideoFiles = result.MediaFiles
	payload.Scan.MatchedFiles = result.MediaFiles
	payload.Scan.Movies = result.MoviesAdded
	return scan.ReportScan(ctx, s.database.Task, id, payload, s.tasks)
}
