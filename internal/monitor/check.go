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

// Check polls due movie and actor subscriptions in small batches with a pause
// between JavDB requests. The scheduler runs checks serially.
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

	now := time.Now()
	movies, err := service.database.Subscription.Query().
		Where(subscription.KindEQ(subscription.KindMovie), subscription.StatusEQ(subscription.StatusWaiting), subscription.NextCheckAtLTE(now)).
		Order(ent.Asc(subscription.FieldNextCheckAt)).Limit(movieBatchSize).All(ctx)
	if err != nil {
		return fmt.Errorf("load due movie subscriptions: %w", err)
	}
	actors, err := service.database.Subscription.Query().
		Where(subscription.KindEQ(subscription.KindActor), subscription.StatusEQ(subscription.StatusActive), subscription.NextCheckAtLTE(now)).
		Order(ent.Asc(subscription.FieldNextCheckAt)).Limit(actorBatchSize).All(ctx)
	if err != nil {
		return fmt.Errorf("load due actor subscriptions: %w", err)
	}

	var checkErrors []error
	for _, record := range movies {
		if err := pace(); err != nil {
			return err
		}
		if err := service.checkMovie(ctx, record, picker, cfg.CheckTime); err != nil {
			if ctx.Err() != nil {
				return err
			}
			checkErrors = append(checkErrors, err)
		}
	}
	for _, record := range actors {
		if err := pace(); err != nil {
			return err
		}
		if err := service.checkActor(ctx, record, cfg.CheckTime); err != nil {
			if ctx.Err() != nil {
				return err
			}
			checkErrors = append(checkErrors, err)
		}
	}
	return errors.Join(checkErrors...)
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
		if stale {
			service.tasks.NotifyMonitorChanged()
		}
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
	if !cursor.isInitialized() {
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
