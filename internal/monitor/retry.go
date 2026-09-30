package monitor

import (
	"context"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

// RetryTask starts a new attempt on the same batch row, containing only failed
// and unprocessed subscriptions. Successful and waiting items are not replayed.
func (s *Service) RetryTask(ctx context.Context, id int) (domain.TaskInfo, error) {
	err := ent.WithTx(ctx, s.database, func(tx *ent.Tx) error {
		record, err := tx.Task.Get(ctx, id)
		if err != nil {
			return err
		}
		if record.Type != string(tasks.KindSubscriptionBatch) {
			return domain.E(domain.KindInvalid, "该任务不是订阅批量任务", nil)
		}
		if record.Status == task.StatusQueued || record.Status == task.StatusRunning {
			return domain.E(domain.KindConflict, "任务正在处理中，无需重复重试", nil)
		}
		payload, err := tasks.DecodePayload[batchPayload](record.Payload)
		if err != nil {
			return err
		}
		if payload.Batch.Failed != len(payload.FailedIDs) {
			return domain.E(domain.KindConflict, "旧任务未保存完整的失败项编号，请重新选择失败的订阅入库", nil)
		}
		remaining := append(payload.FailedIDs, payload.IDs[min(len(payload.IDs), max(0, payload.Batch.Processed)):]...)
		ids := make([]int, 0, len(remaining))
		seen := make(map[int]bool, len(remaining))
		for _, id := range remaining {
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
		if len(ids) == 0 {
			return domain.E(domain.KindConflict, "没有可重试的失败项", nil)
		}
		encoded, err := tasks.EncodePayload(batchPayload{IDs: ids, Batch: domain.SubscriptionBatch{Total: len(ids)}})
		if err != nil {
			return err
		}
		return tx.Task.UpdateOneID(id).SetPayload(encoded).SetStatus(task.StatusQueued).SetProgress(0).ClearError().Exec(ctx)
	})
	if err != nil {
		return domain.TaskInfo{}, err
	}
	s.tasks.NotifyUI()
	s.tasks.WakePool()
	record, err := s.database.Task.Get(ctx, id)
	if err != nil {
		return domain.TaskInfo{}, err
	}
	return batchTaskInfo(record)
}
