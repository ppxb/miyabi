package monitor

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/subscription"
)

type observedDiscoverer struct {
	Discoverer
	before func(kind, id string) error
}

func (d observedDiscoverer) CatalogueMagnets(ctx context.Context, id string) ([]domain.Magnet, error) {
	if err := d.before("movie", id); err != nil {
		return nil, err
	}
	return d.Discoverer.CatalogueMagnets(ctx, id)
}

func (d observedDiscoverer) BrowseMovies(ctx context.Context, options domain.BrowseOptions) ([]domain.Movie, error) {
	if err := d.before("actor", options.EntityID); err != nil {
		return nil, err
	}
	return d.Discoverer.BrowseMovies(ctx, options)
}

func TestCheckDrainsDuePagesWithoutRepeatingFailures(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, ctx := newFixture(t), t.Context()
		start := time.Now()
		const movies, actors = movieBatchSize*2 + 3, actorBatchSize + 2
		var due []*ent.Subscription
		for _, group := range []struct {
			kind   subscription.Kind
			status subscription.Status
			count  int
		}{
			{subscription.KindMovie, subscription.StatusWaiting, movies},
			{subscription.KindActor, subscription.StatusActive, actors},
		} {
			for i := range group.count {
				// Equal deadlines cross page boundaries; older deadlines have larger IDs.
				due = append(due, f.client.Subscription.Create().SetKind(group.kind).SetStatus(group.status).
					SetTargetID(fmt.Sprintf("%s-%02d", group.kind, i)).SetReleaseDate(start.Format(dateLayout)).
					SetNextCheckAt(start.Add(-time.Hour-time.Duration(i/12)*time.Minute)).SaveX(ctx))
			}
		}
		future := f.client.Subscription.Create().SetTargetID("future").SetNextCheckAt(start.Add(time.Second)).SaveX(ctx)
		paused := f.client.Subscription.Create().SetKind(subscription.KindActor).SetStatus(subscription.StatusPaused).
			SetTargetID("paused").SetNextCheckAt(start.Add(-time.Hour)).SaveX(ctx)
		stale := f.client.Subscription.Create().SetStatus(subscription.StatusStale).
			SetTargetID("stale").SetNextCheckAt(start.Add(-time.Hour)).SaveX(ctx)
		// The last record of the first movie page fails to persist its next check.
		failedID := due[21].ID
		writeFailure := errors.New("fixture update failure")
		f.client.Subscription.Use(func(next ent.Mutator) ent.Mutator {
			return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
				if m := mutation.(*ent.SubscriptionMutation); m.Op().Is(ent.OpUpdateOne) {
					if id, _ := m.ID(); id == failedID {
						return nil, writeFailure
					}
				}
				return next.Mutate(ctx, mutation)
			})
		})
		upstreamFailure := errors.New("fixture catalogue failure")
		calls := make(map[string]int)
		var kinds []string
		var last time.Time
		f.service.discover = observedDiscoverer{f.discover, func(kind, id string) error {
			if !last.IsZero() && time.Since(last) < requestGap {
				t.Error("request pacing was lost across a page or subscription kind")
			}
			last = time.Now()
			calls[id]++
			kinds = append(kinds, kind)
			if id == "movie-00" {
				return upstreamFailure
			}
			return nil
		}}
		if err := f.service.Check(ctx); !errors.Is(err, writeFailure) || !errors.Is(err, upstreamFailure) {
			t.Fatalf("check did not retain item failures: %v", err)
		}
		if len(kinds) != movies+actors || kinds[movieBatchSize] != "actor" || kinds[movieBatchSize+actorBatchSize] != "movie" {
			t.Fatalf("backlog was truncated or actors were starved: %v", kinds)
		}
		for _, record := range due {
			if calls[record.TargetID] != 1 {
				t.Fatalf("%s checked %d times", record.TargetID, calls[record.TargetID])
			}
			updated := f.client.Subscription.GetX(ctx, record.ID)
			if record.ID == failedID {
				if updated.Checks != 0 || !updated.NextCheckAt.Equal(*record.NextCheckAt) {
					t.Fatal("failed update unexpectedly changed the row")
				}
			} else if updated.NextCheckAt == nil || !updated.NextCheckAt.After(start) || updated.LastCheckedAt == nil {
				t.Fatalf("due subscription was not rescheduled: %s", record.TargetID)
			}
		}
		for _, record := range []*ent.Subscription{future, paused, stale} {
			if calls[record.TargetID] != 0 || f.client.Subscription.GetX(ctx, record.ID).LastCheckedAt != nil {
				t.Fatalf("ineligible subscription was checked: %s", record.TargetID)
			}
		}
	})
}

func TestCheckCancellationLeavesRemainingSubscriptionsDue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		for i := range movieBatchSize + 1 {
			f.client.Subscription.Create().SetTargetID(fmt.Sprint(i)).SetNextCheckAt(time.Now()).SaveX(t.Context())
		}
		calls := 0
		f.service.discover = observedDiscoverer{f.discover, func(string, string) error { calls++; return nil }}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go func() { time.Sleep(time.Second); cancel() }()
		if err := f.service.Check(ctx); !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatalf("canceled check: calls=%d err=%v", calls, err)
		}
		if err := f.service.Check(t.Context()); err != nil || calls != movieBatchSize+1 {
			t.Fatalf("next run did not resume remaining checks: calls=%d err=%v", calls, err)
		}
		for _, record := range f.client.Subscription.Query().AllX(t.Context()) {
			if record.Checks != 1 {
				t.Fatalf("subscription %d checked %d times", record.ID, record.Checks)
			}
		}
	})
}

func TestCheckNotifiesWhenNoMagnetIsFound(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	record := f.client.Subscription.Create().SetTargetID("waiting").SetReleaseDate(time.Now().Format(dateLayout)).
		SetNextCheckAt(time.Now()).SetError("previous failure").SaveX(ctx)
	revision := f.tasks.Revisions().Monitor
	if err := f.service.Check(ctx); err != nil {
		t.Fatal(err)
	}
	updated := f.client.Subscription.GetX(ctx, record.ID)
	if updated.Status != subscription.StatusWaiting || updated.Checks != 1 || updated.Error != nil || updated.LastCheckedAt == nil || !updated.NextCheckAt.After(time.Now()) {
		t.Fatalf("no-magnet check did not update the waiting subscription: %+v", updated)
	}
	if f.tasks.Revisions().Monitor <= revision {
		t.Fatal("waiting subscription check did not notify the UI")
	}
}
