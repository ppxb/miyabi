package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/subscription"
)

type AddActorOptions struct {
	Title        string
	Cover        string
	AutoDownload *bool
	Zone         string
}

func (service *Service) existingActor(ctx context.Context, actorID string) (Item, bool, error) {
	existing, err := service.database.Subscription.Query().
		Where(subscription.KindEQ(subscription.KindActor), subscription.TargetIDEQ(actorID)).Only(ctx)
	if ent.IsNotFound(err) {
		return Item{}, false, nil
	}
	if err != nil {
		return Item{}, false, fmt.Errorf("find actor subscription: %w", err)
	}
	if existing.Status != subscription.StatusPaused {
		return subscriptionItem(existing), true, nil
	}
	item, err := service.Update(ctx, existing.ID, UpdateOptions{Status: ptr(string(StatusActive))})
	return item, true, err
}

// AddActor subscribes to an actor. The current works page becomes the cursor
// baseline, unreleased works are tracked right away, and later releases
// arrive through the daily check. Re-adding a paused subscription resumes it.
func (service *Service) AddActor(ctx context.Context, actorID string, opts AddActorOptions) (Item, error) {
	if item, found, err := service.existingActor(ctx, actorID); err != nil || found {
		return item, err
	}

	movies, err := service.browseActor(ctx, actorID)
	if err != nil {
		return Item{}, err
	}
	title, cover := opts.Title, opts.Cover
	for _, movie := range movies {
		for _, actor := range movie.Actors {
			if actor.ID != actorID {
				continue
			}
			if title == "" {
				title = actor.Name
			}
			if cover == "" {
				cover = actor.Avatar
			}
		}
	}
	if title == "" {
		title = actorID
	}

	cfg := service.config(ctx)
	autoDownload := cfg.ActorAutoDownload
	if opts.AutoDownload != nil {
		autoDownload = *opts.AutoDownload
	}
	now := time.Now()
	today := now.Format(dateLayout)
	record, err := service.database.Subscription.Create().
		SetKind(subscription.KindActor).SetTargetID(actorID).SetTitle(title).SetCover(cover).
		SetAutoDownload(autoDownload).SetStatus(subscription.StatusActive).SetZone(opts.Zone).
		SetCursor(snapshotCursor(movies, today).encode()).SetNextCheckAt(nextDaily(now, cfg.CheckTime)).Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			if item, found, findErr := service.existingActor(ctx, actorID); findErr == nil && found {
				return item, nil
			}
		}
		return Item{}, fmt.Errorf("create actor subscription: %w", err)
	}
	for _, movie := range movies {
		if movie.ReleaseDate == "" || movie.ReleaseDate >= today {
			service.spawnMovie(ctx, record, movie)
		}
	}
	service.tasks.NotifyMonitorChanged()
	return subscriptionItem(record), nil
}

func (service *Service) browseActor(ctx context.Context, actorID string) ([]domain.Movie, error) {
	movies, err := service.discover.BrowseMovies(ctx, domain.BrowseOptions{
		EntityType: domain.EntityActor, EntityID: actorID,
		Sort: "release", Order: "desc", Page: 1, Limit: actorFeedPageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("browse actor %s works: %w", actorID, err)
	}
	return movies, nil
}

// spawnMovie tracks one work of an actor subscription, inheriting its
// auto-download and zone. Failures are logged; one bad title must not stop
// the actor check.
func (service *Service) spawnMovie(ctx context.Context, origin *ent.Subscription, movie domain.Movie) {
	summary := domain.MovieSummary{ID: movie.ID, Code: movie.Code, Title: movie.Title, Cover: movie.Cover, ReleaseDate: movie.ReleaseDate}
	opts := AddMovieOptions{AutoDownload: &origin.AutoDownload, Zone: origin.Zone, OriginID: &origin.ID}
	if _, err := service.addMovie(ctx, summary, opts); err != nil {
		slog.WarnContext(ctx, "actor subscription could not track a work", "actor", origin.Title, "code", movie.Code, "error", err)
	}
}
