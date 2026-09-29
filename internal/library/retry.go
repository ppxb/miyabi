package library

import (
	"context"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

// RetryTask resumes a failed scan or retries only its failed metadata children.
// Keep IDs and payloads so checkpoints, cached artwork and offline links survive.
func (s *Service) RetryTask(ctx context.Context, id int) (domain.TaskInfo, error) {
	parent, err := s.database.Task.Get(ctx, id)
	if err != nil {
		return domain.TaskInfo{}, err
	}
	if parent.Type != string(tasks.KindScan) {
		return domain.TaskInfo{}, domain.E(domain.KindInvalid, "该任务不是扫描任务", nil)
	}
	input, err := tasks.DecodePayload[domain.ScanPayload](parent.Payload)
	if err != nil {
		return domain.TaskInfo{}, err
	}
	commit := func(fn func(*ent.Tx) error) error { return ent.WithTx(ctx, s.database, fn) }
	if input.Source.AccountID == "local" {
		root, err := s.localScanRoot()
		if err != nil {
			return domain.TaskInfo{}, err
		}
		if root != input.Source.Directory.ID {
			return domain.TaskInfo{}, domain.E(domain.KindConflict, "Emby 本地目录已变更，请扫描当前目录", nil)
		}
	} else {
		sess, err := s.drive.OpenSource(ctx, input.Source)
		if err != nil {
			return domain.TaskInfo{}, err
		}
		commit = func(fn func(*ent.Tx) error) error { return sess.Commit(ctx, fn) }
	}
	err = commit(func(tx *ent.Tx) error {
		current, err := tx.Task.Get(ctx, id)
		if err != nil {
			return err
		}
		if current.Status == task.StatusQueued || current.Status == task.StatusRunning {
			return domain.E(domain.KindConflict, "任务正在处理中，无需重复重试", nil)
		}
		count, err := tx.Task.Update().Where(task.TypeIn(string(tasks.KindScrape), string(tasks.KindCover)),
			task.StatusEQ(task.StatusFailed), func(selector *sql.Selector) {
				selector.Where(sqljson.ValueEQ(task.FieldPayload, id, sqljson.Path("scan_task_id")))
			}).SetStatus(task.StatusQueued).SetProgress(0).ClearError().Save(ctx)
		if err != nil {
			return err
		}
		if current.Status == task.StatusFailed {
			return tx.Task.UpdateOneID(id).SetStatus(task.StatusQueued).SetProgress(0).ClearError().Exec(ctx)
		}
		if count == 0 {
			return domain.E(domain.KindConflict, "没有可重试的失败项", nil)
		}
		return nil
	})
	if err != nil {
		return domain.TaskInfo{}, err
	}
	s.tasks.NotifyLibraryChanged()
	s.tasks.NotifyOfflineChanged()
	parent, err = s.database.Task.Get(ctx, id)
	if err != nil {
		return domain.TaskInfo{}, err
	}
	infos, err := s.Workflows(ctx, []*ent.Task{parent})
	if err != nil {
		return domain.TaskInfo{}, err
	}
	return infos[0], nil
}
