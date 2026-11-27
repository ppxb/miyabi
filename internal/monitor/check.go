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
// the works no followed actor owns any more. A subscription that still maps to
// a library actor is kept even before its source ids are resolved, so a fresh
// follow survives until the background check records them.
func (service *Service) dropUntracked(ctx context.Context) error {
	untracked, err := service.database.Subscription.Query().
		Where(subscription.KindEQ(subscription.KindActor), subscription.FanzaIDEQ(""), subscription.JavdbIDEQ("")).All(ctx)
	if err != nil {
		return fmt.Errorf("query untracked subscriptions: %w", err)
	}
	legacy := make([]int, 0, len(untracked))
	for _, record := range untracked {
		_, lookupErr := service.actors(ctx, record.TargetID)
		if lookupErr == nil {
			continue
		}
		// Only a definitive lookup answer means the row cannot be checked;
		// a transient failure must not drop a fresh follow.
		if domain.IsKind(lookupErr, domain.KindNotFound) || domain.IsKind(lookupErr, domain.KindInvalid) {
			legacy = append(legacy, record.ID)
		}
	}
	removed := 0
	err = ent.WithTx(ctx, service.database, func(tx *ent.Tx) error {
		if len(legacy) > 0 {
			deleted, err := tx.Subscription.Delete().
				Where(subscription.KindEQ(subscription.KindActor), subscription.IDIn(legacy...)).Exec(ctx)
			if err != nil {
				return err
			}
			removed += deleted
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
		removed += works
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
	cursor := decodeCursor(record.Cursor)
	fanzaID, javdbID, err := service.resolveActorSources(ctx, record, !cursor.Initialized)
	if err != nil {
		return service.deferCheck(ctx, record, now, err)
	}
	if fanzaID != record.FanzaID || javdbID != record.JavdbID {
		if err := record.Update().SetFanzaID(fanzaID).SetJavdbID(javdbID).Exec(ctx); err != nil && !ent.IsNotFound(err) {
			return service.deferCheck(ctx, record, now, fmt.Errorf("update actor subscription %d sources: %w", record.ID, err))
		}
	}
	movies, err := service.actorWorks(ctx, fanzaID, javdbID)
	if err != nil {
		return service.deferCheck(ctx, record, now, domain.E(domain.KindUpstream, "查询演员新作失败："+domain.PublicMessage(err), err))
	}
	today := now.Format(dateLayout)
	if !cursor.Initialized {
		// First check: take the page as the baseline and track works released
		// since the follow right away, without replaying the back catalogue.
		// The follow date matters when this first check is delayed.
		since := record.CreatedAt.Format(dateLayout)
		cursor = snapshotCursor(movies, today)
		for _, movie := range movies {
			if movie.ReleaseDate == "" || movie.ReleaseDate >= since {
				service.spawnMovie(ctx, record, movie)
			}
		}
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

// resolveActorSources resolves the ids an actor's works are listed by. The
// library actor supplies the source ids and name spellings known locally;
// FANZA is asked by name only on the first check, so a subscription that
// started on JavDB never switches sources and mixes their id namespaces.
func (service *Service) resolveActorSources(ctx context.Context, record *ent.Subscription, first bool) (fanzaID, javdbID string, err error) {
	fanzaID, javdbID = record.FanzaID, record.JavdbID
	if fanzaID != "" || (!first && javdbID != "") {
		return fanzaID, javdbID, nil
	}
	identity, lookupErr := service.actors(ctx, record.TargetID)
	if lookupErr != nil {
		if javdbID == "" {
			return "", "", lookupErr
		}
		// The stored JavDB id still works when the library actor is gone.
		slog.WarnContext(ctx, "library actor lookup failed; checking on JavDB", "subscription", record.ID, "error", lookupErr)
		return "", javdbID, nil
	}
	if javdbID == "" {
		javdbID = identity.JavdbID
	}
	if identity.FanzaID != "" {
		return identity.FanzaID, javdbID, nil
	}
	resolved, resolveErr := service.fanza.ResolveActress(ctx, identity.Names)
	if resolveErr != nil {
		if javdbID == "" {
			return "", "", domain.E(domain.KindUpstream, "查询演员新作失败："+domain.PublicMessage(resolveErr), resolveErr)
		}
		slog.WarnContext(ctx, "FANZA lookup failed; checking on JavDB", "actor", identity.Name, "error", resolveErr)
		return "", javdbID, nil
	}
	if resolved == "" && javdbID == "" {
		return "", "", domain.E(domain.KindInvalid, "FANZA 和 JavDB 都找不到这位演员，无法追踪新作", nil)
	}
	return resolved, javdbID, nil
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
