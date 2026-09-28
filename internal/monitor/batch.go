package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/subscription"
	"github.com/ppxb/miyabi/internal/magnet"
	"github.com/ppxb/miyabi/internal/tasks"
)

const (
	// Batches pause a random 1.5 to 3 seconds between 115 submissions to stay
	// under its rate limits.
	batchGapMin       = 1500 * time.Millisecond
	batchGapJitter    = 1500 * time.Millisecond
	batchFailureLimit = 20
)

// EnqueueSingle ingests one movie subscription: pick the best magnet by
// preference and submit it to 115. Without a qualifying magnet the
// subscription keeps waiting with auto-download switched on.
func (service *Service) EnqueueSingle(ctx context.Context, id int) (Item, error) {
	record, err := service.database.Subscription.Get(ctx, id)
	if err != nil {
		return Item{}, err
	}
	if record.Kind != subscription.KindMovie {
		return Item{}, domain.E(domain.KindInvalid, "只有影片订阅可以入库", nil)
	}
	if record.Status == subscription.StatusAdded {
		return subscriptionItem(record), nil
	}
	cfg := service.config(ctx)
	magnets, err := service.discover.CatalogueMagnets(ctx, record.TargetID)
	if err != nil {
		return Item{}, fmt.Errorf("fetch magnets for %s: %w", record.Code, err)
	}
	now := time.Now()
	best, found := magnet.NewPicker(cfg.Preferences).Pick(magnets)
	if !found {
		updated, err := record.Update().SetAutoDownload(true).SetStatus(subscription.StatusWaiting).
			SetLastCheckedAt(now).SetNextCheckAt(nextDaily(now, cfg.CheckTime)).ClearError().Save(ctx)
		if err != nil {
			return Item{}, fmt.Errorf("update subscription %d: %w", id, err)
		}
		service.tasks.NotifyMonitorChanged()
		return subscriptionItem(updated), nil
	}
	submission, err := service.offline.Add(ctx, record.TargetID, best.Hash)
	if err != nil {
		return Item{}, fmt.Errorf("submit %s to 115: %w", record.Code, err)
	}
	updated, err := record.Update().SetStatus(subscription.StatusAdded).SetHash(best.Hash).SetTaskID(submission.TaskID).
		SetLastCheckedAt(now).AddChecks(1).ClearNextCheckAt().ClearError().Save(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("complete subscription %d: %w", id, err)
	}
	service.tasks.NotifyMonitorChanged()
	return subscriptionItem(updated), nil
}

type BatchEnqueueRequest struct {
	IDs []int
	All bool
}

// EnqueueBatch queues one task that ingests the selected subscriptions, or
// every waiting and stale movie subscription, one at a time.
func (service *Service) EnqueueBatch(ctx context.Context, req BatchEnqueueRequest) (int, error) {
	ids := req.IDs
	if req.All {
		var err error
		ids, err = service.database.Subscription.Query().
			Where(subscription.KindEQ(subscription.KindMovie), subscription.StatusIn(subscription.StatusWaiting, subscription.StatusStale)).
			Order(ent.Asc(subscription.FieldID)).IDs(ctx)
		if err != nil {
			return 0, fmt.Errorf("load pending subscriptions: %w", err)
		}
	}
	if len(ids) == 0 {
		return 0, domain.E(domain.KindInvalid, "没有可入库的订阅", nil)
	}
	return service.enqueueSubscriptionBatch(ctx, ids)
}

// BatchHandler runs a subscription batch task on the single-worker pool.
func (service *Service) BatchHandler(ctx context.Context, job tasks.Job) error {
	payload, err := tasks.DecodePayload[batchPayload](job.Payload)
	if err != nil {
		return err
	}
	start := min(len(payload.IDs), max(0, payload.Batch.Processed))
	for index := start; index < len(payload.IDs); index++ {
		if index > start {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(batchGapMin + rand.N(batchGapJitter)):
			}
		}
		id := payload.IDs[index]
		code := ""
		if record, err := service.database.Subscription.Get(ctx, id); err == nil {
			code = record.Code
		}
		item, err := service.EnqueueSingle(ctx, id)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		payload.Batch.Processed++
		switch {
		case err != nil:
			payload.Batch.Failed++
			if len(payload.Batch.Failures) < batchFailureLimit {
				payload.Batch.Failures = append(payload.Batch.Failures, domain.SubscriptionFailure{Code: code, Error: domain.PublicMessage(err)})
			}
			slog.WarnContext(ctx, "batch ingestion item failed", "subscription_id", id, "code", code, "error", err)
		case item.Status == StatusAdded:
			payload.Batch.Submitted++
		default:
			payload.Batch.Waiting++
		}
		if err := service.saveSubscriptionBatch(ctx, job.ID, payload); err != nil {
			slog.WarnContext(ctx, "batch ingestion progress not saved", "task_id", job.ID, "error", err)
		}
	}
	return nil
}

// BatchFinished bumps the offline and subscription revisions once the batch commits.
func (service *Service) BatchFinished(context.Context, *ent.Tx, tasks.Job, error) (tasks.Change, error) {
	return tasks.ChangeOffline | tasks.ChangeMonitor, nil
}
