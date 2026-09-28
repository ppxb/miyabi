package monitor

import (
	"context"
	"fmt"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

// batchPayload is the stored JSON payload of a subscription batch
// task: the subscription IDs to process and the running tally.
type batchPayload struct {
	IDs   []int                    `json:"ids"`
	Batch domain.SubscriptionBatch `json:"batch"`
}

// enqueueSubscriptionBatch queues one batch task over the given subscriptions.
func (s *Service) enqueueSubscriptionBatch(ctx context.Context, ids []int) (int, error) {
	payload, err := tasks.EncodePayload(batchPayload{IDs: ids, Batch: domain.SubscriptionBatch{Total: len(ids)}})
	if err != nil {
		return 0, err
	}
	record, err := s.database.Task.Create().SetType(string(tasks.KindSubscriptionBatch)).SetPayload(payload).Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("queue subscription batch: %w", err)
	}
	s.tasks.Notify()
	return record.ID, nil
}

// saveSubscriptionBatch stores the tally and derived progress of a running
// batch and notifies task subscribers.
func (s *Service) saveSubscriptionBatch(ctx context.Context, id int, payload batchPayload) error {
	encoded, err := tasks.EncodePayload(payload)
	if err != nil {
		return err
	}
	progress := 0
	if payload.Batch.Total > 0 {
		progress = min(100, max(0, payload.Batch.Processed*100/payload.Batch.Total))
	}
	if err := s.database.Task.UpdateOneID(id).SetProgress(progress).SetPayload(encoded).Exec(ctx); err != nil {
		return fmt.Errorf("save subscription batch %d: %w", id, err)
	}
	s.tasks.Notify()
	return nil
}

// batchTaskInfo projects a subscription batch task for the API.
func batchTaskInfo(record *ent.Task) (domain.TaskInfo, error) {
	payload, err := tasks.DecodePayload[batchPayload](record.Payload)
	if err != nil {
		return domain.TaskInfo{}, fmt.Errorf("read subscription batch task %d: %w", record.ID, err)
	}
	batch := payload.Batch
	return domain.TaskInfo{
		ID: record.ID, Type: record.Type, Status: string(record.Status), Progress: record.Progress,
		Error: record.Error, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		Batch: &batch,
	}, nil
}

const recentBatchTasks = 5

// ListTasks returns the most recent batch tasks plus any still active.
func (s *Service) ListTasks(ctx context.Context) ([]domain.TaskInfo, error) {
	database := s.database
	recent, err := database.Task.Query().Where(task.TypeEQ(string(tasks.KindSubscriptionBatch))).
		Order(ent.Desc(task.FieldID)).Limit(recentBatchTasks).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list subscription batch tasks: %w", err)
	}
	active, err := database.Task.Query().Where(task.TypeEQ(string(tasks.KindSubscriptionBatch)),
		task.StatusIn(task.StatusQueued, task.StatusRunning)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active subscription batch tasks: %w", err)
	}
	seen := make(map[int]bool, len(recent))
	records := recent
	for _, record := range recent {
		seen[record.ID] = true
	}
	for _, record := range active {
		if !seen[record.ID] {
			records = append(records, record)
		}
	}
	result := make([]domain.TaskInfo, 0, len(records))
	for _, record := range records {
		info, err := batchTaskInfo(record)
		if err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	return result, nil
}
