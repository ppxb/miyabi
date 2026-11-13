package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/library/scan"
)

// startLocalScan queues an import of the configured local media directory.
func (s *Service) startLocalScan(ctx context.Context) (domain.TaskInfo, error) {
	root, err := s.localRoot(ctx)
	if err != nil {
		return domain.TaskInfo{}, err
	}
	return s.EnqueueScan(ctx, localSource(root))
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

// configuredLocalRoot returns the explicitly bound local media directory.
func (s *Service) configuredLocalRoot(ctx context.Context) (string, error) {
	dir, found, err := database.LoadSetting[string](ctx, s.database, domain.SettingLocalDirectory)
	if err != nil {
		return "", err
	}
	if !found || strings.TrimSpace(dir) == "" {
		return "", nil
	}
	return canonicalDirectory(dir)
}

// localRoot prefers the bound local directory and otherwise uses the Emby
// export directory as the default local scan root.
func (s *Service) localRoot(ctx context.Context) (string, error) {
	if root, err := s.configuredLocalRoot(ctx); err != nil {
		return "", err
	} else if root != "" {
		return root, nil
	}
	dir := s.exportMgr.Config().EmbyDir
	if dir == "" {
		return "", domain.E(domain.KindInvalid, "未配置本地媒体目录", nil)
	}
	return canonicalDirectory(dir)
}

func localSource(root string) domain.LibrarySource {
	return domain.LibrarySource{
		AccountID: domain.LocalAccountID,
		Directory: domain.LibraryDirectory{ID: root, Name: "本地媒体目录", Path: root},
	}
}

func canonicalDirectory(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}

func (s *Service) scanLocal(ctx context.Context, id int, payload domain.ScanPayload) error {
	root, err := s.localRoot(ctx)
	if err != nil {
		return err
	}
	if root != payload.Source.Directory.ID {
		return domain.E(domain.KindConflict, "本地媒体目录已变更，请重新扫描", nil)
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
