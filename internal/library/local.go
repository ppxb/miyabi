package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/library/scan"
)

// StartLocalScan queues an import of the configured Emby directory. A legacy
// explicit path is accepted only when it names that same directory.
func (s *Service) StartLocalScan(ctx context.Context, requested string) (domain.TaskInfo, error) {
	root, err := s.localScanRoot()
	if err != nil {
		return domain.TaskInfo{}, err
	}
	if requested != "" {
		path, err := canonicalDirectory(requested)
		if err != nil || path != root {
			return domain.TaskInfo{}, domain.E(domain.KindInvalid, "只能扫描当前配置的 Emby 本地目录", nil)
		}
	}
	return s.EnqueueScan(ctx, domain.LibrarySource{
		AccountID: "local",
		Directory: domain.LibraryDirectory{ID: root, Name: "Emby 本地目录", Path: root},
	})
}

// ScheduleLocalScan also runs at startup, where a fresh install may not have
// created its export directory yet. No media needs importing in that case.
func (s *Service) ScheduleLocalScan(ctx context.Context) error {
	_, err := s.StartLocalScan(ctx, "")
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

func canonicalDirectory(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}

func (s *Service) scanLocal(ctx context.Context, id int, payload domain.ScanPayload) error {
	root, err := s.localScanRoot()
	if err != nil {
		return err
	}
	if root != payload.Source.Directory.ID {
		return domain.E(domain.KindConflict, "Emby 本地目录已变更，请扫描当前目录", nil)
	}
	payload.Scan = domain.ScanProgress{Stage: "scanning", CurrentPath: root}
	if err := scan.ReportScan(ctx, s.database.Task, id, payload, s.tasks); err != nil {
		return err
	}
	result, err := s.localScanner.Scan(ctx, root)
	if err != nil {
		return fmt.Errorf("scan Emby directory: %w", err)
	}
	payload.Scan.Stage = "done"
	payload.Scan.FilesScanned = result.FilesScanned
	payload.Scan.VideoFiles = result.MediaFiles
	payload.Scan.MatchedFiles = result.MediaFiles
	payload.Scan.Movies = result.MoviesAdded
	return scan.ReportScan(ctx, s.database.Task, id, payload, s.tasks)
}
