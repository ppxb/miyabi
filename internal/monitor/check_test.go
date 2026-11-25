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
	f.identities["actor-1"] = ActorIdentity{Name: "Yua Mikami", Avatar: "/api/actors/1/avatar", Names: []string{"Yua Mikami"}, JavdbID: "actor-1"}
	actor, err := f.service.AddActor(ctx, "actor-1")
	if err != nil || actor.Title != "Yua Mikami" || actor.Cover == "" || actor.Status != subscription.StatusActive || actor.NextCheckAt == nil || !actor.NextCheckAt.After(time.Now()) {
		t.Fatalf("AddActor: %#v %v", actor, err)
	}
	feed, err := f.service.ActorFeed(ctx, actor.ID, 1, 10)
	if err != nil || len(feed) != 1 || feed[0].TargetID != "act-upcoming" || feed[0].OriginID == nil || *feed[0].OriginID != actor.ID {
		t.Fatalf("only the unreleased work is tracked at subscription time: %#v %v", feed, err)
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
	if _, err := f.service.AddActor(ctx, "actor-1"); err == nil {
		t.Fatal("an actor subscription without a cursor baseline must not be created")
	}
	if list, _ := f.service.List(ctx, "actor", 1, 10); len(list) != 0 {
		t.Fatalf("expected no actor subscription, got %v", list)
	}
}

func TestCheckDefersUpstreamFailure(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	item, _ := f.service.AddActor(ctx, "actor-1")
	f.forceDue(t, item.ID)
	f.discover.browseErr = domain.E(domain.KindUpstream, "JavDB 暂不可用", nil)
	revision := f.tasks.Revisions().Monitor
	if err := f.service.Check(ctx); err == nil {
		t.Fatal("Check must surface the deferred cause")
	}
	record := f.client.Subscription.GetX(ctx, item.ID)
	if record.Status != subscription.StatusActive || record.Error == nil || record.NextCheckAt == nil || record.NextCheckAt.After(time.Now().Add(retryInterval)) {
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

	actor, err := f.service.AddActor(ctx, "actor-zero")
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

func TestFollowListsWorksOnFANZAFirst(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	f.discover.browseErr = errors.New("JavDB must not be asked")
	f.identities["7"] = ActorIdentity{Name: "河北彩花", Names: []string{"河北彩花", "河北彩伽"}, JavdbID: "ZY5eq"}
	f.fanza.ids["河北彩伽"] = "1044864"
	f.fanza.works["1044864"] = []domain.Movie{{ID: "SNOS-535", Code: "SNOS-535", ReleaseDate: "2099-12-21"}}
	actor, err := f.service.AddActor(ctx, "7")
	if err != nil {
		t.Fatal(err)
	}
	record := f.client.Subscription.GetX(ctx, actor.ID)
	if record.TargetID != "7" || record.FanzaID != "1044864" || record.JavdbID != "ZY5eq" || record.Title != "河北彩花" {
		t.Fatalf("follow must store the library key and both source ids: %+v", record)
	}
	if feed, _ := f.service.ActorFeed(ctx, actor.ID, 1, 10); len(feed) != 1 || feed[0].TargetID != "SNOS-535" {
		t.Fatalf("works must come from FANZA: %#v", feed)
	}
	f.forceDue(t, actor.ID)
	if err := f.service.Check(ctx); err != nil {
		t.Fatalf("checks must keep using FANZA: %v", err)
	}
}

func TestFollowNeedsASource(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	f.identities["8"] = ActorIdentity{Name: "無名", Names: []string{"無名"}}
	if _, err := f.service.AddActor(ctx, "8"); !domain.IsKind(err, domain.KindInvalid) {
		t.Fatalf("an actor no source knows cannot be followed: %v", err)
	}
	if count := f.client.Subscription.Query().CountX(ctx); count != 0 {
		t.Fatalf("a failed follow left %d subscriptions", count)
	}
}

func TestCheckDropsUntrackedSubscriptions(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	// Recorded by JavDB actor id before following moved to library actors.
	legacy := f.client.Subscription.Create().SetKind(subscription.KindActor).SetStatus(subscription.StatusActive).
		SetTargetID("ZY5eq").SaveX(ctx)
	f.client.Subscription.Create().SetTargetID("OLD-001").SetOriginID(legacy.ID).SaveX(ctx)
	f.client.Subscription.Create().SetTargetID("MANUAL-001").SaveX(ctx)
	f.discover.actorMovies["9"] = []domain.Movie{{ID: "NEW-001", Code: "NEW-001", ReleaseDate: "2099-01-01"}}
	kept, err := f.service.AddActor(ctx, "9")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.Check(ctx); err != nil {
		t.Fatal(err)
	}
	var left []string
	for _, record := range f.client.Subscription.Query().AllX(ctx) {
		left = append(left, record.TargetID)
	}
	if len(left) != 2 || !f.client.Subscription.Query().Where(subscription.IDEQ(kept.ID)).ExistX(ctx) {
		t.Fatalf("only the tracked actor and its work may remain, got %v", left)
	}
}

func TestFollowFallsBackToJavDBWhenFANZAFails(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	f.fanza.resolveErr = errors.New("FANZA unreachable")
	f.discover.actorMovies["ZY5eq"] = []domain.Movie{{ID: "NEW-001", Code: "NEW-001", ReleaseDate: "2099-01-01"}}
	f.identities["7"] = ActorIdentity{Name: "河北彩花", Names: []string{"河北彩花"}, JavdbID: "ZY5eq"}
	actor, err := f.service.AddActor(ctx, "7")
	if err != nil {
		t.Fatalf("an actor with a JavDB id must still be followed: %v", err)
	}
	if record := f.client.Subscription.GetX(ctx, actor.ID); record.FanzaID != "" || record.JavdbID != "ZY5eq" {
		t.Fatalf("follow must fall back to JavDB: %+v", record)
	}
	f.identities["8"] = ActorIdentity{Name: "無名", Names: []string{"無名"}}
	if _, err := f.service.AddActor(ctx, "8"); err == nil {
		t.Fatal("without a JavDB id the FANZA failure must surface")
	}
}

func TestFollowStoresTheCanonicalKey(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	f.identities["05"] = ActorIdentity{Key: "5", Name: "5", Names: []string{"5"}, JavdbID: "5"}
	first, err := f.service.AddActor(ctx, "05")
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.service.AddActor(ctx, "5")
	if err != nil || second.ID != first.ID || first.TargetID != "5" {
		t.Fatalf("spellings of one key must share a subscription: %#v %#v %v", first, second, err)
	}
}
