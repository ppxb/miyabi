package tasks

import (
	"context"
	"fmt"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
)

// SubscriptionBatchPayload is the stored JSON payload of a subscription batch
// task: the subscription IDs to process and the running tally.
type SubscriptionBatchPayload struct {
	IDs   []int                    `json:"ids"`
	Batch domain.SubscriptionBatch `json:"batch"`
}

// EnqueueSubscriptionBatch queues one batch task over the given subscriptions.
func (s *Service) EnqueueSubscriptionBatch(ctx context.Context, ids []int) (TaskInfo, error) {
	payload, err := EncodePayload(SubscriptionBatchPayload{IDs: ids, Batch: domain.SubscriptionBatch{Total: len(ids)}})
	if err != nil {
		return TaskInfo{}, err
	}
	record, err := s.database.Task.Create().SetType(string(KindSubscriptionBatch)).SetPayload(payload).Save(ctx)
	if err != nil {
		return TaskInfo{}, fmt.Errorf("queue subscription batch: %w", err)
	}
	s.bus.Notify()
	return BatchTaskInfo(record)
}

// SaveSubscriptionBatch stores the tally and derived progress of a running
// batch and notifies task subscribers.
func (s *Service) SaveSubscriptionBatch(ctx context.Context, id int, payload SubscriptionBatchPayload) error {
	encoded, err := EncodePayload(payload)
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
	s.bus.Notify()
	return nil
}

// BatchTaskInfo projects a subscription batch task for the API.
func BatchTaskInfo(record *ent.Task) (TaskInfo, error) {
	payload, err := DecodePayload[SubscriptionBatchPayload](record.Payload)
	if err != nil {
		return TaskInfo{}, fmt.Errorf("read subscription batch task %d: %w", record.ID, err)
	}
	batch := payload.Batch
	return TaskInfo{
		ID: record.ID, Type: record.Type, Status: record.Status, Progress: record.Progress,
		Error: record.Error, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		Batch: &batch,
	}, nil
}

// listBatchTasks returns the most recent batch tasks plus any still active.
func listBatchTasks(ctx context.Context, database *ent.Client) ([]TaskInfo, error) {
	recent, err := database.Task.Query().Where(task.TypeEQ(string(KindSubscriptionBatch))).
		Order(ent.Desc(task.FieldID)).Limit(recentBatchTasks).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list subscription batch tasks: %w", err)
	}
	active, err := database.Task.Query().Where(task.TypeEQ(string(KindSubscriptionBatch)),
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
	result := make([]TaskInfo, 0, len(records))
	for _, record := range records {
		info, err := BatchTaskInfo(record)
		if err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	return result, nil
}
