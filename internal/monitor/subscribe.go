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
	// OriginID marks a subscription spawned by an actor subscription.
	OriginID *int
}

// addMovie records a new work found by an actor check. Movies are never
// subscribed directly; an existing subscription is left untouched.
func (service *Service) addMovie(ctx context.Context, summary domain.MovieSummary, opts AddMovieOptions) (Item, error) {
	if item, found, err := service.existingMovie(ctx, summary.ID); err != nil || found {
		return item, err
	}
	return service.createMovie(ctx, summary, opts)
}

func (service *Service) createMovie(ctx context.Context, summary domain.MovieSummary, opts AddMovieOptions) (Item, error) {
	record, err := service.database.Subscription.Create().
		SetKind(subscription.KindMovie).SetTargetID(summary.ID).SetCode(summary.Code).
		SetTitle(summary.Title).SetCover(summary.Cover).SetReleaseDate(summary.ReleaseDate).
		SetNillableOriginID(opts.OriginID).Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			// Lost a race with a concurrent add; query the existing row once.
			if item, found, findErr := service.existingMovie(ctx, summary.ID); findErr == nil && found {
				return item, nil
			}
		}
		return Item{}, fmt.Errorf("create movie subscription: %w", err)
	}
	service.tasks.NotifyMonitorChanged()
	return subscriptionItem(record), nil
}

// existingMovie finds the subscription already recorded for a movie.
func (service *Service) existingMovie(ctx context.Context, movieID string) (Item, bool, error) {
	existing, err := service.database.Subscription.Query().
		Where(subscription.KindEQ(subscription.KindMovie), subscription.TargetIDEQ(movieID)).Only(ctx)
	if ent.IsNotFound(err) {
		return Item{}, false, nil
	}
	if err != nil {
		return Item{}, false, fmt.Errorf("find subscription: %w", err)
	}
	return subscriptionItem(existing), true, nil
}

type UpdateOptions struct {
	// Status pauses or resumes an actor subscription.
	Status *string
}

func (service *Service) Update(ctx context.Context, id int, opts UpdateOptions) (Item, error) {
	record, err := service.database.Subscription.Get(ctx, id)
	if err != nil {
		return Item{}, err
	}
	update := record.Update()
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

// Remove deletes a subscription; a missing ID surfaces as not found. A work is
// recorded once under the first actor that found it, so each work of a removed
// actor moves to another followed actor who has seen it, or is deleted.
func (service *Service) Remove(ctx context.Context, id int) error {
	if err := ent.WithTx(ctx, service.database, func(tx *ent.Tx) error {
		works, err := tx.Subscription.Query().Where(subscription.OriginIDEQ(id)).All(ctx)
		if err != nil {
			return err
		}
		if len(works) > 0 {
			others, err := tx.Subscription.Query().
				Where(subscription.KindEQ(subscription.KindActor), subscription.IDNEQ(id)).All(ctx)
			if err != nil {
				return err
			}
			seenBy := make(map[string]int)
			for _, other := range others {
				for _, movieID := range decodeCursor(other.Cursor).SeenMovieIDs {
					seenBy[movieID] = other.ID
				}
			}
			for _, work := range works {
				if owner, ok := seenBy[work.TargetID]; ok {
					err = tx.Subscription.UpdateOneID(work.ID).SetOriginID(owner).Exec(ctx)
				} else {
					err = tx.Subscription.DeleteOneID(work.ID).Exec(ctx)
				}
				if err != nil {
					return err
				}
			}
		}
		return tx.Subscription.DeleteOneID(id).Exec(ctx)
	}); err != nil {
		return err
	}
	service.tasks.NotifyMonitorChanged()
	return nil
}
