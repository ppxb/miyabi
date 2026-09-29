package monitor

import (
	"errors"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/subscription"
)

func (f fixture) forceDue(t *testing.T, id int) {
	t.Helper()
	if err := f.client.Subscription.UpdateOneID(id).SetNextCheckAt(time.Now().Add(-time.Hour)).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestActorSubscriptionSpawnsOnlyNewWorks(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	actorRef := []domain.Actor{{ID: "actor-1", Name: "Yua Mikami", Avatar: "https://example.com/avatar.jpg"}}
	f.discover.actorMovies["actor-1"] = []domain.Movie{
		{ID: "act-upcoming", Code: "UP-001", Title: "Upcoming", ReleaseDate: "2099-01-01", Actors: actorRef},
		{ID: "act-past", Code: "PAST-001", Title: "Past", ReleaseDate: "2020-01-01", Actors: actorRef},
	}
	actor, err := f.service.AddActor(ctx, "actor-1", AddActorOptions{})
	if err != nil || actor.Title != "Yua Mikami" || actor.Cover == "" || actor.Status != subscription.StatusActive || actor.NextCheckAt == nil || !actor.NextCheckAt.After(time.Now()) {
		t.Fatalf("AddActor: %#v %v", actor, err)
	}
	feed, err := f.service.ActorFeed(ctx, actor.ID, 1, 10)
	if err != nil || len(feed) != 1 || feed[0].TargetID != "act-upcoming" || feed[0].OriginID == nil || *feed[0].OriginID != actor.ID || feed[0].AutoDownload {
		t.Fatalf("only the unreleased work is tracked at subscription time: %#v %v", feed, err)
	}
	if f.discover.summaryCalls != 0 {
		t.Fatalf("spawned works must not cost detail requests, got %d", f.discover.summaryCalls)
	}

	f.discover.actorMovies["actor-1"] = append([]domain.Movie{
		{ID: "act-new", Code: "NEW-001", ReleaseDate: "2099-02-01"},
		{ID: "act-late", Code: "LATE-001", ReleaseDate: "2020-01-10"},
		{ID: "act-backfill", Code: "OLD-001", ReleaseDate: "2010-01-01"},
	}, f.discover.actorMovies["actor-1"]...)
	f.forceDue(t, actor.ID)
	if err := f.service.Check(ctx); err != nil {
		t.Fatalf("Check: %v", err)
	}
	feed, _ = f.service.ActorFeed(ctx, actor.ID, 1, 10)
	if len(feed) != 3 || feed[0].TargetID != "act-new" || feed[1].TargetID != "act-upcoming" || feed[2].TargetID != "act-late" {
		t.Fatalf("expected the new release and the late-indexed title but not the backfill, got %v", feed)
	}

	f.forceDue(t, actor.ID)
	if err := f.service.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if feed, _ = f.service.ActorFeed(ctx, actor.ID, 1, 10); len(feed) != 3 {
		t.Fatalf("an unchanged page spawns nothing, got %d", len(feed))
	}
}

func TestAddActorRequiresWorksPage(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	f.discover.browseErr = errors.New("javdb down")
	if _, err := f.service.AddActor(ctx, "actor-1", AddActorOptions{Title: "Someone"}); err == nil {
		t.Fatal("an actor subscription without a cursor baseline must not be created")
	}
	if list, _ := f.service.List(ctx, "actor", 1, 10); len(list) != 0 {
		t.Fatalf("expected no actor subscription, got %v", list)
	}
}

func TestCheckMovieWithoutAutoDownloadWaitsUntilTomorrow(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	item, _ := f.service.AddMovie(ctx, "m1", AddMovieOptions{AutoDownload: ptr(false)})
	f.discover.magnets["m1"] = []domain.Magnet{{Hash: "h1", Name: "MOCK-m1", HasSubtitle: true}}
	if err := f.service.Check(ctx); err != nil {
		t.Fatal(err)
	}
	record := f.client.Subscription.GetX(ctx, item.ID)
	if record.Status != subscription.StatusWaiting || record.Hash != "h1" || record.NextCheckAt == nil || !record.NextCheckAt.After(time.Now()) || len(f.offline.submissions) != 0 {
		t.Fatalf("manual subscriptions remember the pick and wait for tomorrow: %#v submissions=%d", record, len(f.offline.submissions))
	}

	if _, err := f.service.Update(ctx, item.ID, UpdateOptions{AutoDownload: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	f.forceDue(t, item.ID)
	if err := f.service.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if record = f.client.Subscription.GetX(ctx, item.ID); record.Status != subscription.StatusAdded || len(f.offline.submissions) != 1 {
		t.Fatalf("with auto-download the same check submits: %#v submissions=%d", record, len(f.offline.submissions))
	}
}

func TestCheckDefersUpstreamFailure(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	item, _ := f.service.AddMovie(ctx, "m1", AddMovieOptions{})
	f.discover.magnetsErr = domain.E(domain.KindUpstream, "JavDB 暂不可用", nil)
	revision := f.tasks.Revisions().Monitor
	if err := f.service.Check(ctx); err == nil {
		t.Fatal("Check must surface the deferred cause")
	}
	record := f.client.Subscription.GetX(ctx, item.ID)
	if record.Status != subscription.StatusWaiting || record.Error == nil || record.NextCheckAt == nil || record.NextCheckAt.After(time.Now().Add(retryInterval)) {
		t.Fatalf("a failed check retries within the hour: %#v", record)
	}
	if f.tasks.Revisions().Monitor == revision {
		t.Fatal("subscription changes must notify subscribers")
	}
}

func TestActorSubscriptionZeroInitialWorksSpawnsFirstWorks(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	actorRef := []domain.Actor{{ID: "actor-zero", Name: "Zero Works Debut", Avatar: "https://example.com/avatar.jpg"}}
	// Actor initially has 0 works on JavDB
	f.discover.actorMovies["actor-zero"] = []domain.Movie{}

	actor, err := f.service.AddActor(ctx, "actor-zero", AddActorOptions{Title: "Zero Works Debut"})
	if err != nil {
		t.Fatalf("AddActor: %v", err)
	}

	// Verify no movies spawned initially
	feed, err := f.service.ActorFeed(ctx, actor.ID, 1, 10)
	if err != nil || len(feed) != 0 {
		t.Fatalf("expected 0 works initially, got %d %v", len(feed), err)
	}

	// Later, actor's debut works appear on JavDB
	f.discover.actorMovies["actor-zero"] = []domain.Movie{
		{ID: "act-debut-1", Code: "DEBUT-001", Title: "Debut 1", ReleaseDate: "2099-01-01", Actors: actorRef},
		{ID: "act-debut-2", Code: "DEBUT-002", Title: "Debut 2", ReleaseDate: "2026-09-01", Actors: actorRef},
	}

	f.forceDue(t, actor.ID)
	if err := f.service.Check(ctx); err != nil {
		t.Fatalf("Check: %v", err)
	}

	// Verify both debut works were spawned as subscriptions
	feed, err = f.service.ActorFeed(ctx, actor.ID, 1, 10)
	if err != nil || len(feed) != 2 {
		t.Fatalf("expected 2 spawned works for debut actor, got %d (err: %v)", len(feed), err)
	}
	targetIDs := map[string]bool{feed[0].TargetID: true, feed[1].TargetID: true}
	if !targetIDs["act-debut-1"] || !targetIDs["act-debut-2"] {
		t.Fatalf("expected act-debut-1 and act-debut-2 in feed, got %#v", feed)
	}
}
