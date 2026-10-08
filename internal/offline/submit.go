package offline

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/pan"
)

// submit handles duplicate history by inspecting its real output. Only a
// terminal task with confirmed absent video content is removed, never files.
// The caller holds the account/hash lock, never the shared Pan state lock.
func (service *Service) submit(ctx context.Context, sess drive.Session, hash string) (pan.OfflineTask, error) {
	infoHash, err := sess.AddOffline(ctx, "magnet:?xt=urn:btih:"+hash)
	if err == nil {
		return pan.OfflineTask{Hash: infoHash}, nil
	}
	if !errors.Is(err, pan.ErrOfflineExists) {
		return pan.OfflineTask{}, err
	}
	remote, err := service.findRemoteTask(ctx, sess, hash)
	if err != nil {
		return pan.OfflineTask{}, err
	}
	source := sess.Source()
	if remote.Status == 0 || remote.Status == 1 {
		if remote.DirectoryID != source.Directory.ID {
			return pan.OfflineTask{}, domain.E(domain.KindConflict, "115 已有该磁力的下载任务，目标目录与当前媒体目录不一致", nil)
		}
		return remote, nil
	}
	if remote.Status != 2 && remote.Status != -1 {
		return pan.OfflineTask{}, fmt.Errorf("115 returned unknown offline status %d", remote.Status)
	}
	if remote.FileID == "" {
		return pan.OfflineTask{}, domain.E(domain.KindConflict, "115 的历史任务未提供资源位置，请先在 115 客户端清理该任务记录", nil)
	}
	present, err := service.remoteHasVideo(ctx, sess, remote.FileID)
	if err != nil {
		return pan.OfflineTask{}, err
	}
	if present {
		if remote.Status == -1 {
			return pan.OfflineTask{}, domain.E(domain.KindConflict, "115 任务失败但目录内仍有视频，请先在 115 客户端确认完整性", nil)
		}
		return remote, nil
	}
	if err := sess.RemoveOffline(ctx, remote.Hash); err != nil {
		return pan.OfflineTask{}, fmt.Errorf("remove stale 115 task history: %w", err)
	}
	infoHash, err = sess.AddOffline(ctx, "magnet:?xt=urn:btih:"+hash)
	return pan.OfflineTask{Hash: infoHash}, err
}

func (service *Service) findRemoteTask(ctx context.Context, sess drive.Session, hash string) (pan.OfflineTask, error) {
	remote, found, err := service.lookupRemoteTask(ctx, sess, hash)
	if err != nil {
		return pan.OfflineTask{}, err
	}
	if found {
		return remote, nil
	}
	return pan.OfflineTask{}, domain.E(domain.KindBusy, "115 提示任务已存在，但任务列表中未找到它，请稍后重试", nil)
}

func (service *Service) lookupRemoteTask(ctx context.Context, sess drive.Session, hash string) (pan.OfflineTask, bool, error) {
	var found *pan.OfflineTask
	err := drive.WalkOfflinePages(ctx, func(page int) (pan.OfflinePage, error) {
		return sess.OfflineTasks(ctx, page)
	}, func(remote pan.OfflinePage) (bool, error) {
		for _, download := range remote.Tasks {
			if strings.EqualFold(download.Hash, hash) {
				found = &download
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		return pan.OfflineTask{}, false, fmt.Errorf("find 115 task: %w", err)
	}
	if found != nil {
		return *found, true, nil
	}
	return pan.OfflineTask{}, false, nil
}

func (service *Service) remoteHasVideo(ctx context.Context, sess drive.Session, id string) (bool, error) {
	info, err := sess.Info(ctx, id)
	if errors.Is(err, pan.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check existing 115 resource: %w", err)
	}
	source := sess.Source()
	if !drive.WithinSource(info, source) {
		return false, domain.E(domain.KindConflict, "该磁力的资源已在媒体目录之外，请先在 115 中移动资源", nil)
	}
	if !info.IsDirectory {
		return domain.IsVideo(info.Name), nil
	}
	directories := []string{info.ID}
	seen := map[string]bool{info.ID: true}
	found := false
	for next := 0; next < len(directories) && !found; next++ {
		dirID := directories[next]
		err := drive.WalkFilePages(ctx, func(offset int) (pan.FilePage, error) {
			return sess.List(ctx, dirID, offset)
		}, func(page pan.FilePage) (bool, error) {
			if !slices.ContainsFunc(page.Path, func(dir pan.Directory) bool { return dir.ID == source.Directory.ID }) {
				return false, domain.E(domain.KindNotFound, "下载目录已移出媒体目录", nil)
			}
			for _, entry := range page.Files {
				if entry.IsDirectory {
					if !seen[entry.ID] {
						seen[entry.ID] = true
						directories = append(directories, entry.ID)
					}
				} else if domain.IsVideo(entry.Name) {
					found = true
					return false, nil
				}
			}
			return true, nil
		})
		if err != nil {
			return false, fmt.Errorf("check downloaded video files: %w", err)
		}
	}
	return found, nil
}
