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
	item, err := service.Update(ctx, existing.ID, UpdateOptions{Status: new(string(subscription.StatusActive))})
	return item, true, err
}

// AddActor follows a library actor by key. Their works are listed on FANZA,
// or on JavDB when FANZA does not know them; the current works page becomes
// the cursor baseline, unreleased works are tracked right away, and later
// releases arrive through the daily check. Re-adding a paused subscription
// resumes it.
func (service *Service) AddActor(ctx context.Context, actorKey string) (Item, error) {
	identity, err := service.actors(ctx, actorKey)
	if err != nil {
		return Item{}, err
	}
	// Subscriptions are stored under the canonical key, whatever spelling came in.
	actorKey = identity.Key
	if item, found, err := service.existingActor(ctx, actorKey); err != nil || found {
		return item, err
	}
	fanzaID := identity.FanzaID
	if fanzaID == "" {
		if fanzaID, err = service.fanza.ResolveActress(ctx, identity.Names); err != nil {
			if identity.JavdbID == "" {
				return Item{}, fmt.Errorf("find %s on FANZA: %w", identity.Name, err)
			}
			// FANZA being unreachable or ambiguous must not block the JavDB fallback.
			slog.WarnContext(ctx, "FANZA lookup failed; following on JavDB", "actor", identity.Name, "error", err)
			fanzaID = ""
		}
	}
	if fanzaID == "" && identity.JavdbID == "" {
		return Item{}, domain.E(domain.KindInvalid, "FANZA 和 JavDB 都找不到这位演员，无法追踪新作", nil)
	}
	movies, err := service.actorWorks(ctx, fanzaID, identity.JavdbID)
	if err != nil {
		return Item{}, err
	}

	cfg := service.config(ctx)
	now := time.Now()
	today := now.Format(dateLayout)
	record, err := service.database.Subscription.Create().
		SetKind(subscription.KindActor).SetTargetID(actorKey).SetTitle(identity.Name).SetCover(identity.Avatar).
		SetFanzaID(fanzaID).SetJavdbID(identity.JavdbID).SetStatus(subscription.StatusActive).
		SetCursor(snapshotCursor(movies, today).encode()).SetNextCheckAt(nextDaily(now, cfg.CheckTime)).Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			if item, found, findErr := service.existingActor(ctx, actorKey); findErr == nil && found {
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

// actorWorks lists an actor's newest works from FANZA, or from JavDB when the
// actor has no FANZA id.
func (service *Service) actorWorks(ctx context.Context, fanzaID, javdbID string) ([]domain.Movie, error) {
	if fanzaID != "" {
		movies, err := service.fanza.ActressWorks(ctx, fanzaID)
		if err != nil {
			return nil, fmt.Errorf("list FANZA actress %s works: %w", fanzaID, err)
		}
		return movies, nil
	}
	movies, err := service.discover.BrowseMovies(ctx, domain.BrowseOptions{
		EntityType: domain.EntityActor, EntityID: javdbID,
		Sort: "release", Order: "desc", Page: 1, Limit: actorFeedPageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("browse JavDB actor %s works: %w", javdbID, err)
	}
	return movies, nil
}

// spawnMovie records one new work of an actor subscription. Failures are
// logged; one bad title must not stop the actor check.
func (service *Service) spawnMovie(ctx context.Context, origin *ent.Subscription, movie domain.Movie) {
	summary := domain.MovieSummary{ID: movie.ID, Code: movie.Code, Title: movie.Title, Cover: movie.Cover, ReleaseDate: movie.ReleaseDate}
	opts := AddMovieOptions{OriginID: &origin.ID}
	if _, err := service.addMovie(ctx, summary, opts); err != nil {
		slog.WarnContext(ctx, "actor subscription could not track a work", "actor", origin.Title, "code", movie.Code, "error", err)
	}
}
