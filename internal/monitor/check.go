package monitor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/subscription"
	"github.com/ppxb/miyabi/internal/magnet"
)

const (
	retryInterval     = time.Hour
	movieBatchSize    = 10
	actorBatchSize    = 5
	requestGap        = 2 * time.Second
	actorFeedPageSize = 40
)

// Check drains subscriptions due at the start of this run in small, alternating
// movie and actor pages. The scheduler runs checks serially, with paced requests.
func (service *Service) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	cfg := service.config(ctx)
	picker := magnet.NewPicker(cfg.Preferences)
	requests := 0
	pace := func() error {
		if requests > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(requestGap):
			}
		}
		requests++
		return nil
	}

	cutoff := time.Now()
	groups := []struct {
		kind   subscription.Kind
		status subscription.Status
		limit  int
		after  *ent.Subscription
		done   bool
	}{
		{kind: subscription.KindMovie, status: subscription.StatusWaiting, limit: movieBatchSize},
		{kind: subscription.KindActor, status: subscription.StatusActive, limit: actorBatchSize},
	}
	var checkErrors []error
	for {
		more := false
		for i := range groups {
			group := &groups[i]
			if group.done {
				continue
			}
			records, err := service.dueSubscriptions(ctx, group.kind, group.status, cutoff, group.after, group.limit)
			if err != nil {
				return errors.Join(append(checkErrors, err)...)
			}
			group.done = len(records) < group.limit
			more = more || !group.done
			if len(records) > 0 {
				group.after = records[len(records)-1]
			}
			for _, record := range records {
				if err := pace(); err != nil {
					return err
				}
				if group.kind == subscription.KindMovie {
					err = service.checkMovie(ctx, record, picker, cfg.CheckTime)
				} else {
					err = service.checkActor(ctx, record, cfg.CheckTime)
				}
				if err != nil {
					if ctx.Err() != nil {
						return err
					}
					checkErrors = append(checkErrors, err)
				}
			}
		}
		if !more {
			break
		}
	}
	return errors.Join(checkErrors...)
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

func (service *Service) checkMovie(ctx context.Context, record *ent.Subscription, picker *magnet.Picker, checkTime string) error {
	now := time.Now()
	magnets, err := service.discover.CatalogueMagnets(ctx, record.TargetID)
	if err != nil {
		return service.deferCheck(ctx, record, now, domain.E(domain.KindUpstream, "查询磁力失败："+domain.PublicMessage(err), err))
	}
	best, found := picker.Pick(magnets)
	if !found {
		next, stale := nextMovieCheck(now, record.ReleaseDate, record.CreatedAt, checkTime)
		update := record.Update().SetLastCheckedAt(now).AddChecks(1).ClearError()
		if stale {
			update.SetStatus(subscription.StatusStale).ClearNextCheckAt()
		} else {
			update.SetNextCheckAt(next)
		}
		if err := update.Exec(ctx); err != nil && !ent.IsNotFound(err) {
			return fmt.Errorf("schedule subscription %d: %w", record.ID, err)
		}
		service.tasks.NotifyMonitorChanged()
		return nil
	}
	if !record.AutoDownload {
		// Remember the pick for the user and look again tomorrow.
		if err := record.Update().SetHash(best.Hash).SetLastCheckedAt(now).SetNextCheckAt(nextDaily(now, checkTime)).
			AddChecks(1).ClearError().Exec(ctx); err != nil && !ent.IsNotFound(err) {
			return fmt.Errorf("update subscription %d: %w", record.ID, err)
		}
		service.tasks.NotifyMonitorChanged()
		return nil
	}
	submission, err := service.offline.Add(ctx, record.TargetID, best.Hash)
	if err != nil {
		return service.deferCheck(ctx, record, now, domain.E(domain.KindUpstream, "加入 115 失败："+domain.PublicMessage(err), err))
	}
	if err := record.Update().SetStatus(subscription.StatusAdded).SetHash(best.Hash).SetTaskID(submission.TaskID).
		SetLastCheckedAt(now).AddChecks(1).ClearNextCheckAt().ClearError().Exec(ctx); err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("complete subscription %d: %w", record.ID, err)
	}
	slog.InfoContext(ctx, "subscribed movie submitted to 115", "code", record.Code, "hash", best.Hash, "pan_task_id", submission.TaskID)
	service.tasks.NotifyMonitorChanged()
	return nil
}

func (service *Service) checkActor(ctx context.Context, record *ent.Subscription, checkTime string) error {
	now := time.Now()
	movies, err := service.browseActor(ctx, record.TargetID)
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
