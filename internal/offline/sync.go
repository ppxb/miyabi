package offline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/offlinedownload"
	"github.com/ppxb/miyabi/internal/pan"
)

// Sync polls active and pending 115 offline tasks and updates their state in the database.
// The scheduler runs syncs serially; per-magnet locks coordinate with submissions.
func (service *Service) Sync(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	source := service.drive.Source()
	if source == nil {
		return nil
	}
	sess, err := service.drive.OpenSource(ctx, *source)
	if err != nil {
		if errors.Is(err, drive.ErrMediaDirectoryRequired) {
			return nil
		}
		return err
	}

	records, err := service.database.OfflineDownload.Query().Where(
		offlinedownload.AccountIDEQ(source.AccountID),
		offlinedownload.Or(offlinedownload.StatusEQ(offlinedownload.StatusRunning),
			offlinedownload.And(offlinedownload.StatusEQ(offlinedownload.StatusDone),
				offlinedownload.DirectoryIDEQ(source.Directory.ID), offlinedownload.ScanTaskIDEQ(0),
				offlinedownload.Or(offlinedownload.FileIDNEQ(""), offlinedownload.AwaitingLocationEQ(true))))).
		Order(ent.Desc(offlinedownload.FieldID)).All(ctx)
	if err != nil {
		return fmt.Errorf("load offline tasks: %w", err)
	}
	if len(records) == 0 {
		return service.Recover(ctx)
	}

	wanted := make(map[string]*ent.OfflineDownload)
	seen := make(map[string]bool)
	var syncErrors []error
	for _, record := range records {
		if record.Recovery != nil && record.Recovery.Action != "" {
			continue
		}
		hash := strings.ToLower(record.Hash)
		if seen[hash] {
			continue
		}
		seen[hash] = true
		if record.Status == offlinedownload.StatusDone && record.FileID != "" {
			if err := service.UpdateTask(ctx, sess, record, pan.OfflineTask{Status: 2, FileID: record.FileID, Hash: record.InfoHash}); err != nil {
				syncErrors = append(syncErrors, err)
			}
			continue
		}
		wanted[strings.ToLower(record.InfoHash)] = record
	}

	if len(wanted) > 0 {
		if err := drive.WalkOfflinePages(ctx, func(page int) (pan.OfflinePage, error) {
			return sess.OfflineTasks(ctx, page)
		}, func(remote pan.OfflinePage) (bool, error) {
			for _, download := range remote.Tasks {
				if err := ctx.Err(); err != nil {
					return false, err
				}
				key := strings.ToLower(download.Hash)
				record, ok := wanted[key]
				if !ok {
					continue
				}
				// A task we found is never missing, even if its update fails.
				// Keep syncing other tasks and retry this one on the next poll.
				delete(wanted, key)
				if err := service.UpdateTask(ctx, sess, record, download); err != nil {
					if errors.Is(err, drive.ErrSourceChanged) || errors.Is(err, pan.ErrUnauthorized) {
						return false, err
					}
					syncErrors = append(syncErrors, err)
				}
			}
			return len(wanted) > 0, nil
		}); err != nil {
			// Do not mark unseen tasks missing after an incomplete listing.
			return errors.Join(append(syncErrors, fmt.Errorf("sync 115 offline tasks: %w", err))...)
		}
	}

	for _, record := range wanted {
		if err := service.markMissing(ctx, sess, record); err != nil {
			syncErrors = append(syncErrors, err)
		}
	}
	if len(syncErrors) == 0 {
		return service.Recover(ctx)
	}
	return errors.Join(syncErrors...)
}

// UpdateTask applies a 115 offline remote task state to a local offline task.
// The caller has already matched the remote task to the record by info hash.
func (service *Service) UpdateTask(ctx context.Context, sess drive.Session, record *ent.OfflineDownload, remote pan.OfflineTask) error {
	unlock, err := service.lockTask(ctx, record.ID)
	if err != nil {
		return err
	}
	defer unlock()
	_, prefs, err := service.config(ctx)
	if err != nil {
		return err
	}

	notify := false
	err = sess.CommitAccount(ctx, func(tx *ent.Tx) error {
		current, err := tx.OfflineDownload.Get(ctx, record.ID)
		if err != nil {
			return err
		}
		// A remote page may have started loading before another completion
		// committed. Never regress terminal state or overwrite indexed files.
		if current.Hash != record.Hash || current.InfoHash != record.InfoHash ||
			current.Status == offlinedownload.StatusCancelled || current.Status == offlinedownload.StatusFailed || current.Status == offlinedownload.StatusDone && remote.Status != 2 {
			return nil
		}
		if current.Recovery != nil && current.Recovery.Action != "" {
			return nil
		}
		status := offlinedownload.StatusRunning
		switch remote.Status {
		case 0, 1:
		case 2:
			if current.Status == offlinedownload.StatusDone && (current.ScanTaskID != 0 ||
				current.FileID == "" && remote.FileID == "") {
				return nil
			}
			notify = true
			return service.completeTask(ctx, tx, current, remote.FileID)
		case -1:
			status = offlinedownload.StatusFailed
		default:
			return fmt.Errorf("115 returned unknown offline status %d", remote.Status)
		}
		state := recovery(current, prefs)
		now, progress := service.now(), rawProgress(remote)
		if remote.ProgressUnknown {
			progress, remote.Progress = state.Progress, current.Progress
		}
		if remote.ProgressUnknown || state.ObservedAt.IsZero() || now.Sub(state.ObservedAt) > observationGap || progress != state.Progress || remote.Status != state.RemoteStatus {
			state.ProgressAt, state.Stalled = now, false
		}
		state.ObservedAt, state.Progress, state.RemoteStatus = now, progress, remote.Status
		update := tx.OfflineDownload.UpdateOneID(current.ID).SetStatus(status).SetProgress(remote.Progress).SetRecovery(state)
		if current.Recovery != nil && progress > current.Recovery.Progress && status == offlinedownload.StatusRunning {
			state.RetryAt, state.Failures = time.Time{}, 0
			update.ClearError()
		}
		if status == offlinedownload.StatusFailed {
			update.SetError("115 离线下载失败，请在 115 客户端查看原因")
		}
		notify = current.Status != status || current.Progress != remote.Progress || current.Recovery != nil &&
			(current.Recovery.Stalled != state.Stalled || current.Recovery.RemoteStatus != state.RemoteStatus ||
				current.Error != nil && progress > current.Recovery.Progress)
		return update.Exec(ctx)
	})
	if err != nil {
		return fmt.Errorf("update offline task %d: %w", record.ID, err)
	}
	if notify {
		service.tasks.NotifyOfflineChanged()
	}
	return nil
}

func (service *Service) markMissing(ctx context.Context, sess drive.Session, record *ent.OfflineDownload) error {
	unlock, err := service.lockTask(ctx, record.ID)
	if err != nil {
		return err
	}
	defer unlock()

	changed := false
	err = sess.CommitAccount(ctx, func(tx *ent.Tx) error {
		current, err := tx.OfflineDownload.Get(ctx, record.ID)
		if err != nil {
			return err
		}
		if current.Hash != record.Hash || current.InfoHash != record.InfoHash || current.Recovery != nil && current.Recovery.Action != "" {
			return nil
		}
		update := tx.OfflineDownload.UpdateOneID(current.ID)
		switch current.Status {
		case offlinedownload.StatusRunning:
			update.SetStatus(offlinedownload.StatusFailed).SetError("115 中未找到该任务，请在 115 客户端确认下载结果")
		case offlinedownload.StatusDone:
			if !current.AwaitingLocation || current.FileID != "" || current.ScanTaskID != 0 {
				return nil
			}
			update.SetAwaitingLocation(false).SetError("115 已完成下载，但任务记录已移除，无法获取文件位置，请扫描媒体目录确认下载结果")
		default:
			return nil
		}
		changed = true
		return update.Exec(ctx)
	})
	if err == nil && changed {
		service.tasks.NotifyOfflineChanged()
	}
	return err
}

// Completion and targeted scan creation are one transaction. Never merge into
// a running scan: it might already have passed the newly downloaded directory.
func (service *Service) completeTask(ctx context.Context, tx *ent.Tx, record *ent.OfflineDownload, fileID string) error {
	if record.Status == offlinedownload.StatusDone && record.FileID != "" {
		fileID = record.FileID
	}
	scanID := record.ScanTaskID
	source := service.drive.Source()
	if fileID != "" && scanID == 0 && source != nil &&
		source.Directory.ID == record.DirectoryID && source.AccountID == record.AccountID {
		var err error
		scanID, err = service.library.EnqueueTargetedScan(ctx, tx, *source, fileID, record.ID, record.Code, record.JavdbID)
		if err != nil {
			return err
		}
	}
	update := tx.OfflineDownload.UpdateOneID(record.ID).SetStatus(offlinedownload.StatusDone).
		SetProgress(100).ClearError().SetFileID(fileID).SetAwaitingLocation(fileID == "").SetScanTaskID(scanID)
	if record.Recovery != nil {
		state := recovery(record, record.Recovery.Preferences)
		state.Action, state.NextHash, state.Reason = "", "", ""
		state.Stalled, state.Exhausted = false, false
		state.RemoteStatus, state.Progress = 2, 100
		update.SetRecovery(state)
	}
	return update.Exec(ctx)
}
