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
		// Three actor pages; equal deadlines cross page boundaries and older
		// deadlines have larger IDs.
		const actors = actorBatchSize*2 + 3
		var due []*ent.Subscription
		for i := range actors {
			due = append(due, f.client.Subscription.Create().SetKind(subscription.KindActor).SetStatus(subscription.StatusActive).
				SetTargetID(fmt.Sprintf("actor-%02d", i)).SetJavdbID(fmt.Sprintf("actor-%02d", i)).
				SetNextCheckAt(start.Add(-time.Hour-time.Duration(i/6)*time.Minute)).SaveX(ctx))
		}
		// Movie subscriptions only record works and are never checked.
		movie := f.client.Subscription.Create().SetTargetID("movie").SetOriginID(due[0].ID).SetNextCheckAt(start.Add(-time.Hour)).SaveX(ctx)
		future := f.client.Subscription.Create().SetKind(subscription.KindActor).SetStatus(subscription.StatusActive).
			SetTargetID("future").SetJavdbID("future").SetNextCheckAt(start.Add(time.Second)).SaveX(ctx)
		paused := f.client.Subscription.Create().SetKind(subscription.KindActor).SetStatus(subscription.StatusPaused).
			SetTargetID("paused").SetJavdbID("paused").SetNextCheckAt(start.Add(-time.Hour)).SaveX(ctx)
		// The last record of the first page fails to persist its next check.
		failedID := due[actorBatchSize-1].ID
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
		var last time.Time
		f.service.discover = observedDiscoverer{f.discover, func(_, id string) error {
			if !last.IsZero() && time.Since(last) < requestGap {
				t.Error("request pacing was lost across a page")
			}
			last = time.Now()
			calls[id]++
			if id == "actor-00" {
				return upstreamFailure
			}
			return nil
		}}
		if err := f.service.Check(ctx); !errors.Is(err, writeFailure) || !errors.Is(err, upstreamFailure) {
			t.Fatalf("check did not retain item failures: %v", err)
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
		for _, record := range []*ent.Subscription{future, paused, movie} {
			if calls[record.TargetID] != 0 || f.client.Subscription.GetX(ctx, record.ID).LastCheckedAt != nil {
				t.Fatalf("ineligible subscription was checked: %s", record.TargetID)
			}
		}
	})
}

func TestCheckCancellationLeavesRemainingSubscriptionsDue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		for i := range actorBatchSize + 1 {
			f.client.Subscription.Create().SetKind(subscription.KindActor).SetStatus(subscription.StatusActive).
				SetTargetID(fmt.Sprint(i)).SetJavdbID(fmt.Sprint(i)).SetNextCheckAt(time.Now()).SaveX(t.Context())
		}
		calls := 0
		f.service.discover = observedDiscoverer{f.discover, func(string, string) error { calls++; return nil }}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go func() { time.Sleep(time.Second); cancel() }()
		if err := f.service.Check(ctx); !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatalf("canceled check: calls=%d err=%v", calls, err)
		}
		if err := f.service.Check(t.Context()); err != nil || calls != actorBatchSize+1 {
			t.Fatalf("next run did not resume remaining checks: calls=%d err=%v", calls, err)
		}
		for _, record := range f.client.Subscription.Query().AllX(t.Context()) {
			if record.Checks != 1 {
				t.Fatalf("subscription %d checked %d times", record.ID, record.Checks)
			}
		}
	})
}
