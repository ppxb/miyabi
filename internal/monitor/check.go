package monitor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/subscription"
)

const (
	retryInterval     = time.Hour
	actorBatchSize    = 5
	requestGap        = 2 * time.Second
	actorFeedPageSize = 40
)

// Check drains the actor subscriptions due at the start of this run in small
// pages, recording their new works. The scheduler runs checks serially, with
// paced requests.
func (service *Service) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := service.dropUntracked(ctx); err != nil {
		return fmt.Errorf("drop untracked subscriptions: %w", err)
	}
	cfg := service.config(ctx)
	requests := 0
	cutoff := time.Now()
	var after *ent.Subscription
	var checkErrors []error
	for {
		records, err := service.dueSubscriptions(ctx, subscription.KindActor, subscription.StatusActive, cutoff, after, actorBatchSize)
		if err != nil {
			return errors.Join(append(checkErrors, err)...)
		}
		for _, record := range records {
			if requests > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(requestGap):
				}
			}
			requests++
			if err := service.checkActor(ctx, record, cfg.CheckTime); err != nil {
				if ctx.Err() != nil {
					return err
				}
				checkErrors = append(checkErrors, err)
			}
		}
		if len(records) < actorBatchSize {
			return errors.Join(checkErrors...)
		}
		after = records[len(records)-1]
	}
}

// dropUntracked deletes actor subscriptions no source can check, such as those
// recorded by JavDB actor id before following moved to library actors, and
// the works no followed actor owns any more.
func (service *Service) dropUntracked(ctx context.Context) error {
	removed := 0
	err := ent.WithTx(ctx, service.database, func(tx *ent.Tx) error {
		actors, err := tx.Subscription.Delete().Where(subscription.KindEQ(subscription.KindActor),
			subscription.FanzaIDEQ(""), subscription.JavdbIDEQ("")).Exec(ctx)
		if err != nil {
			return err
		}
		owners, err := tx.Subscription.Query().Where(subscription.KindEQ(subscription.KindActor)).IDs(ctx)
		if err != nil {
			return err
		}
		orphan := subscription.OriginIDIsNil()
		if len(owners) > 0 {
			orphan = subscription.Or(orphan, subscription.OriginIDNotIn(owners...))
		}
		works, err := tx.Subscription.Delete().Where(subscription.KindEQ(subscription.KindMovie), orphan).Exec(ctx)
		removed = actors + works
		return err
	})
	if err == nil && removed > 0 {
		service.tasks.NotifyMonitorChanged()
	}
	return err
}

func (service *Service) dueSubscriptions(ctx context.Context, kind subscription.Kind, status subscription.Status, cutoff time.Time, after *ent.Subscription, limit int) ([]*ent.Subscription, error) {
	query := service.database.Subscription.Query().
		Where(subscription.KindEQ(kind), subscription.StatusEQ(status), subscription.NextCheckAtLTE(cutoff))
	if after != nil {
		// Updates remove rows from the due set. A time/ID cursor avoids skipping
		// rows like offset pagination, or rechecking rows whose update failed.
		query.Where(subscription.Or(
			subscription.NextCheckAtGT(*after.NextCheckAt),
			subscription.And(subscription.NextCheckAtEQ(*after.NextCheckAt), subscription.IDGT(after.ID)),
		))
	}
	records, err := query.Order(ent.Asc(subscription.FieldNextCheckAt), ent.Asc(subscription.FieldID)).Limit(limit).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load due %s subscriptions: %w", kind, err)
	}
	return records, nil
}

func (service *Service) checkActor(ctx context.Context, record *ent.Subscription, checkTime string) error {
	now := time.Now()
	movies, err := service.actorWorks(ctx, record.FanzaID, record.JavdbID)
	if err != nil {
		return service.deferCheck(ctx, record, now, domain.E(domain.KindUpstream, "查询演员新作失败："+domain.PublicMessage(err), err))
	}
	today := now.Format(dateLayout)
	cursor := decodeCursor(record.Cursor)
	if !cursor.Initialized {
		// No baseline yet: record the page without treating the back
		// catalogue as new releases.
		cursor = snapshotCursor(movies, today)
	} else {
		for _, movie := range cursor.newWorks(movies) {
			service.spawnMovie(ctx, record, movie)
		}
		cursor = cursor.advance(movies, today)
	}
	if err := record.Update().SetCursor(cursor.encode()).SetLastCheckedAt(now).SetNextCheckAt(nextDaily(now, checkTime)).
		AddChecks(1).ClearError().Exec(ctx); err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("update actor subscription %d: %w", record.ID, err)
	}
	service.tasks.NotifyMonitorChanged()
	return nil
}

// deferCheck records a transient failure and retries within the hour rather
// than consuming the daily slot. The row stores the public message only; the
// returned error keeps the cause for logs.
func (service *Service) deferCheck(ctx context.Context, record *ent.Subscription, now time.Time, cause error) error {
	if err := record.Update().SetLastCheckedAt(now).SetNextCheckAt(now.Add(retryInterval)).
		SetError(domain.PublicMessage(cause)).Exec(ctx); err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("defer subscription %d: %w", record.ID, err)
	}
	service.tasks.NotifyMonitorChanged()
	return fmt.Errorf("subscription %s: %w", record.Code, cause)
}
