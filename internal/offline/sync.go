package offline

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

// Sync polls active and pending 115 offline tasks and updates their state in the database.
func (service *Service) Sync(ctx context.Context) error {
	if err := service.syncing.Lock(ctx); err != nil {
		return err
	}
	defer service.syncing.Unlock()

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

	records, err := service.database.Task.Query().Where(task.TypeEQ(tasks.KindOffline.String()), task.Or(
		task.StatusIn(task.StatusQueued, task.StatusRunning),
		task.And(task.StatusEQ(task.StatusDone), func(s *sql.Selector) {
			s.Where(sql.And(
				sqljson.ValueEQ(task.FieldPayload, source.AccountID, sqljson.Path("account_id")),
				sqljson.ValueEQ(task.FieldPayload, source.Directory.ID, sqljson.Path("directory_id")),
				sql.Not(sqljson.HasKey(task.FieldPayload, sqljson.Path("scan_task_id"))),
				sql.Or(
					sqljson.HasKey(task.FieldPayload, sqljson.Path("file_id")),
					sqljson.ValueEQ(task.FieldPayload, true, sqljson.Path("awaiting_location")),
				),
			))
		}),
	)).Order(ent.Desc(task.FieldID)).All(ctx)
	if err != nil {
		return fmt.Errorf("load offline tasks: %w", err)
	}
	if len(records) == 0 {
		return nil
	}

	wanted := make(map[string]*ent.Task)
	seen := make(map[string]bool)
	var syncErrors []error
	for _, record := range records {
		input, err := tasks.DecodePayload[offlinePayload](record.Payload)
		if err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("read offline task %d: %w", record.ID, err))
			continue
		}
		if input.AccountID != source.AccountID {
			continue
		}
		hash := strings.ToLower(input.Hash)
		if seen[hash] {
			continue
		}
		seen[hash] = true
		if record.Status == task.StatusDone {
			if input.DirectoryID != source.Directory.ID {
				continue
			}
			if input.FileID != "" {
				if err := service.UpdateTask(ctx, sess, record, pan.OfflineTask{Status: 2, FileID: input.FileID, Hash: input.InfoHash}); err != nil {
					syncErrors = append(syncErrors, err)
				}
				continue
			}
		}
		wanted[strings.ToLower(input.InfoHash)] = record
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
	return errors.Join(syncErrors...)
}

// UpdateTask applies a 115 offline remote task state to a local offline task.
func (service *Service) UpdateTask(ctx context.Context, sess drive.Session, record *ent.Task, remote pan.OfflineTask) error {
	input, err := tasks.DecodePayload[offlinePayload](record.Payload)
	if err != nil {
		return err
	}
	unlock, err := service.operations.Lock(ctx, input.AccountID, strings.ToLower(input.Hash))
	if err != nil {
		return err
	}
	defer unlock()

	notify := false
	err = sess.CommitAccount(ctx, func(tx *ent.Tx) error {
		current, err := tx.Task.Get(ctx, record.ID)
		if err != nil {
			return err
		}
		// A remote page may have started loading before another completion
		// committed. Never regress terminal state or overwrite newer payload.
		if current.Status == task.StatusFailed || current.Status == task.StatusDone && remote.Status != 2 {
			return nil
		}
		currentInput, err := tasks.DecodePayload[offlinePayload](current.Payload)
		if err != nil {
			return err
		}
		if currentInput.AccountID != input.AccountID || currentInput.InfoHash != input.InfoHash {
			return fmt.Errorf("offline task identity changed while syncing")
		}
		if remote.Hash != "" && !strings.EqualFold(remote.Hash, currentInput.InfoHash) {
			return fmt.Errorf("115 returned a different offline task than requested")
		}
		status := task.StatusRunning
		switch remote.Status {
		case 0, 1:
		case 2:
			if current.Status == task.StatusDone && (currentInput.ScanTaskID != 0 ||
				currentInput.FileID == "" && remote.FileID == "") {
				return nil
			}
			notify = true
			return service.completeTask(ctx, tx, current, currentInput, remote.FileID, sess)
		case -1:
			status = task.StatusFailed
		default:
			return fmt.Errorf("115 returned unknown offline status %d", remote.Status)
		}
		if current.Status == status && current.Progress == remote.Progress {
			return nil
		}
		update := tx.Task.UpdateOneID(current.ID).SetStatus(status).SetProgress(remote.Progress)
		if status == task.StatusFailed {
			update.SetError("115 离线下载失败，请在 115 客户端查看原因")
		}
		notify = true
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

func (service *Service) markMissing(ctx context.Context, sess drive.Session, record *ent.Task) error {
	input, err := tasks.DecodePayload[offlinePayload](record.Payload)
	if err != nil {
		return err
	}
	unlock, err := service.operations.Lock(ctx, input.AccountID, strings.ToLower(input.Hash))
	if err != nil {
		return err
	}
	defer unlock()

	changed := false
	err = sess.CommitAccount(ctx, func(tx *ent.Tx) error {
		current, err := tx.Task.Get(ctx, record.ID)
		if err != nil {
			return err
		}
		update := tx.Task.UpdateOneID(current.ID)
		switch current.Status {
		case task.StatusQueued, task.StatusRunning:
			update.SetStatus(task.StatusFailed).SetError("115 中未找到该任务，请在 115 客户端确认下载结果")
		case task.StatusDone:
			pending, err := tasks.DecodePayload[offlinePayload](current.Payload)
			if err != nil {
				return err
			}
			if !pending.AwaitingLocation || pending.FileID != "" || pending.ScanTaskID != 0 {
				return nil
			}
			pending.AwaitingLocation = false
			encoded, err := tasks.EncodePayload(pending)
			if err != nil {
				return err
			}
			update.SetPayload(encoded).SetError("115 已完成下载，但任务记录已移除，无法获取文件位置，请扫描媒体目录确认下载结果")
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
func (service *Service) completeTask(ctx context.Context, tx *ent.Tx, record *ent.Task, input offlinePayload, fileID string, sess drive.Session) error {
	// A delayed completion must keep the target already recorded by a newer
	// result, including when its scan was deferred until the mount returns.
	if record.Status == task.StatusDone && input.FileID != "" {
		fileID = input.FileID
	}
	input.FileID = fileID
	input.AwaitingLocation = fileID == ""
	currentSource := service.drive.Source()
	// Save remote completion immediately; a later sync will create the scan
	// once 115 exposes the output location.
	if fileID != "" && input.ScanTaskID == 0 && currentSource != nil &&
		currentSource.Directory.ID == input.DirectoryID && currentSource.AccountID == input.AccountID && service.library != nil {
		scanID, err := service.library.EnqueueTargetedScan(ctx, tx, *currentSource, fileID, record.ID, input.Code, input.JavDBID)
		if err != nil {
			return err
		}
		input.ScanTaskID = scanID
	}
	encoded, err := tasks.EncodePayload(input)
	if err != nil {
		return err
	}
	return tx.Task.UpdateOneID(record.ID).SetStatus(task.StatusDone).SetProgress(100).ClearError().SetPayload(encoded).Exec(ctx)
}
