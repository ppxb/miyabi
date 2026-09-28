package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/subscription"
)

type AddMovieOptions struct {
	AutoDownload *bool
	Zone         string
	// OriginID marks a subscription spawned by an actor subscription.
	OriginID *int
}

// AddMovie subscribes to a movie. Re-adding a waiting subscription is a
// no-op; a user re-adding a finished or stale one re-arms it.
func (service *Service) AddMovie(ctx context.Context, movieID string, opts AddMovieOptions) (Item, error) {
	if item, found, err := service.existingMovie(ctx, movieID, opts); err != nil || found {
		return item, err
	}
	summary, err := service.discover.MovieSummary(ctx, movieID)
	if err != nil {
		return Item{}, err
	}
	return service.addMovie(ctx, summary, opts)
}

// addMovie is AddMovie with the summary already known, which spares actor
// checks one JavDB detail request per new work.
func (service *Service) addMovie(ctx context.Context, summary domain.MovieSummary, opts AddMovieOptions) (Item, error) {
	if item, found, err := service.existingMovie(ctx, summary.ID, opts); err != nil || found {
		return item, err
	}
	var autoDownload bool
	if opts.AutoDownload != nil {
		autoDownload = *opts.AutoDownload
	} else {
		autoDownload = service.config(ctx).MovieAutoDownload
	}
	record, err := service.database.Subscription.Create().
		SetKind(subscription.KindMovie).SetTargetID(summary.ID).SetCode(summary.Code).
		SetTitle(summary.Title).SetCover(summary.Cover).SetReleaseDate(summary.ReleaseDate).
		SetAutoDownload(autoDownload).SetStatus(subscription.StatusWaiting).SetZone(opts.Zone).
		SetNillableOriginID(opts.OriginID).SetNextCheckAt(time.Now()).Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			// Lost a race with a concurrent add; query the existing row once.
			if item, found, findErr := service.existingMovie(ctx, summary.ID, opts); findErr == nil && found {
				return item, nil
			}
		}
		return Item{}, fmt.Errorf("create movie subscription: %w", err)
	}
	service.tasks.NotifyMonitorChanged()
	service.signal()
	return subscriptionItem(record), nil
}

// existingMovie resolves a duplicate add. Actor-spawned additions never touch
// an existing subscription, so a movie that is already added stays added.
func (service *Service) existingMovie(ctx context.Context, movieID string, opts AddMovieOptions) (Item, bool, error) {
	existing, err := service.database.Subscription.Query().
		Where(subscription.KindEQ(subscription.KindMovie), subscription.TargetIDEQ(movieID)).Only(ctx)
	if ent.IsNotFound(err) {
		return Item{}, false, nil
	}
	if err != nil {
		return Item{}, false, fmt.Errorf("find subscription: %w", err)
	}
	if opts.OriginID != nil || existing.Status == subscription.StatusWaiting {
		return subscriptionItem(existing), true, nil
	}
	item, err := service.retry(ctx, existing)
	return item, true, err
}

func (service *Service) retry(ctx context.Context, record *ent.Subscription) (Item, error) {
	updated, err := record.Update().SetStatus(subscription.StatusWaiting).SetNextCheckAt(time.Now()).
		SetHash("").ClearTaskID().ClearError().Save(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("retry subscription %d: %w", record.ID, err)
	}
	service.tasks.NotifyMonitorChanged()
	service.signal()
	return subscriptionItem(updated), nil
}

type UpdateOptions struct {
	AutoDownload *bool
	Zone         *string
	// Status pauses or resumes an actor subscription; movie status only moves
	// through checks and enqueues.
	Status *string
}

func (service *Service) Update(ctx context.Context, id int, opts UpdateOptions) (Item, error) {
	record, err := service.database.Subscription.Get(ctx, id)
	if err != nil {
		return Item{}, err
	}
	update := record.Update()
	if opts.AutoDownload != nil {
		update.SetAutoDownload(*opts.AutoDownload)
	}
	if opts.Zone != nil {
		update.SetZone(*opts.Zone)
	}
	if opts.Status != nil {
		status := subscription.Status(*opts.Status)
		if record.Kind != subscription.KindActor || (status != subscription.StatusActive && status != subscription.StatusPaused) {
			return Item{}, domain.E(domain.KindInvalid, "只有演员订阅可以暂停或恢复", nil)
		}
		update.SetStatus(status)
		if status == subscription.StatusActive {
			update.SetNextCheckAt(nextDaily(time.Now(), service.config(ctx).CheckTime)).ClearError()
		}
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("update subscription %d: %w", id, err)
	}
	service.tasks.NotifyMonitorChanged()
	return subscriptionItem(updated), nil
}

// Remove deletes a subscription; a missing ID surfaces as not found.
func (service *Service) Remove(ctx context.Context, id int) error {
	if err := service.database.Subscription.DeleteOneID(id).Exec(ctx); err != nil {
		return err
	}
	service.tasks.NotifyMonitorChanged()
	return nil
}

func ptr[T any](value T) *T { return &value }
