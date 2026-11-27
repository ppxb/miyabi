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
	if err != nil || actor.Title != "Yua Mikami" || actor.Cover == "" || actor.Status != subscription.StatusActive || actor.NextCheckAt == nil {
		t.Fatalf("AddActor: %#v %v", actor, err)
	}
	if feed, err := f.service.ActorFeed(ctx, actor.ID, 1, 10); err != nil || len(feed) != 0 {
		t.Fatalf("following must not fetch the works page: %#v %v", feed, err)
	}
	if err := f.service.Check(ctx); err != nil {
		t.Fatalf("first check: %v", err)
	}
	feed, err := f.service.ActorFeed(ctx, actor.ID, 1, 10)
	if err != nil || len(feed) != 1 || feed[0].TargetID != "act-upcoming" || feed[0].OriginID == nil || *feed[0].OriginID != actor.ID {
		t.Fatalf("the first check tracks only the unreleased work: %#v %v", feed, err)
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

func TestFollowDefersTheWorksPageToTheBackgroundCheck(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	f.discover.browseErr = errors.New("javdb down")
	actor, err := f.service.AddActor(ctx, "actor-1")
	if err != nil {
		t.Fatalf("following must not depend on the works page: %v", err)
	}
	if feed, _ := f.service.ActorFeed(ctx, actor.ID, 1, 10); len(feed) != 0 {
		t.Fatalf("the first works page loads in the background, got %v", feed)
	}
	if err := f.service.Check(ctx); err == nil {
		t.Fatal("the background check must surface the upstream failure")
	}
	record := f.client.Subscription.GetX(ctx, actor.ID)
	if record.Error == nil || record.Status != subscription.StatusActive {
		t.Fatalf("a failed first check must stay followed with its error: %#v", record)
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
	// The first check records the empty page as the baseline.
	if err := f.service.Check(ctx); err != nil {
		t.Fatalf("first check: %v", err)
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

func TestCheckListsWorksOnFANZAFirst(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	f.discover.browseErr = errors.New("JavDB must not be asked")
	f.identities["7"] = ActorIdentity{Name: "河北彩花", Names: []string{"河北彩花", "河北彩伽"}, JavdbID: "ZY5eq"}
	f.fanza.ids["河北彩伽"] = "1044864"
	f.fanza.works["1044864"] = []domain.Movie{{ID: "SNOS-535", Code: "SNOS-535", ReleaseDate: "2099-12-21"}}
	actor, err := f.service.AddActor(ctx, "7")
	if err != nil {
		t.Fatal(err)
	}
	if record := f.client.Subscription.GetX(ctx, actor.ID); record.TargetID != "7" || record.JavdbID != "ZY5eq" || record.Title != "河北彩花" {
		t.Fatalf("follow must store the library key and known source ids: %+v", record)
	}
	if err := f.service.Check(ctx); err != nil {
		t.Fatalf("the check must resolve FANZA and use it first: %v", err)
	}
	record := f.client.Subscription.GetX(ctx, actor.ID)
	if record.FanzaID != "1044864" {
		t.Fatalf("the check must record the resolved FANZA id: %+v", record)
	}
	if feed, _ := f.service.ActorFeed(ctx, actor.ID, 1, 10); len(feed) != 1 || feed[0].TargetID != "SNOS-535" {
		t.Fatalf("works must come from FANZA: %#v", feed)
	}
	f.fanza.resolveErr = errors.New("a stored FANZA id must not be resolved again")
	f.forceDue(t, actor.ID)
	if err := f.service.Check(ctx); err != nil {
		t.Fatalf("later checks must reuse the stored FANZA id: %v", err)
	}
}

func TestCheckReportsAnActorNoSourceKnows(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	f.identities["8"] = ActorIdentity{Name: "無名", Names: []string{"無名"}}
	actor, err := f.service.AddActor(ctx, "8")
	if err != nil {
		t.Fatalf("following must not wait on source resolution: %v", err)
	}
	if err := f.service.Check(ctx); err == nil {
		t.Fatal("an actor no source knows must fail the background check")
	}
	record := f.client.Subscription.GetX(ctx, actor.ID)
	if record.Error == nil || record.Status != subscription.StatusActive {
		t.Fatalf("the subscription must stay followed with its error: %#v", record)
	}
}

func TestCheckDropsUntrackedSubscriptions(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	// Recorded by JavDB actor id before following moved to library actors.
	f.lookupErrors["ZY5eq"] = domain.E(domain.KindNotFound, "演员不存在", nil)
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

func TestCheckKeepsSubscriptionsWhenTheActorLookupFails(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	f.lookupErrors["temp-actor"] = errors.New("database is busy")
	record := f.client.Subscription.Create().SetKind(subscription.KindActor).
		SetStatus(subscription.StatusActive).SetTargetID("temp-actor").SaveX(ctx)
	if err := f.service.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if !f.client.Subscription.Query().Where(subscription.IDEQ(record.ID)).ExistX(ctx) {
		t.Fatal("a transient lookup failure must not drop the subscription")
	}
}

func TestCheckFallsBackToJavDBWhenFANZAFails(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	f.fanza.resolveErr = errors.New("FANZA unreachable")
	f.discover.actorMovies["ZY5eq"] = []domain.Movie{{ID: "NEW-001", Code: "NEW-001", ReleaseDate: "2099-01-01"}}
	f.identities["7"] = ActorIdentity{Name: "河北彩花", Names: []string{"河北彩花"}, JavdbID: "ZY5eq"}
	actor, err := f.service.AddActor(ctx, "7")
	if err != nil {
		t.Fatalf("an actor with a JavDB id must still be followed: %v", err)
	}
	if err := f.service.Check(ctx); err != nil {
		t.Fatalf("the check must fall back to JavDB: %v", err)
	}
	if record := f.client.Subscription.GetX(ctx, actor.ID); record.FanzaID != "" || record.JavdbID != "ZY5eq" {
		t.Fatalf("the check must keep using JavDB: %+v", record)
	}
	// FANZA becoming resolvable must not switch an established JavDB baseline:
	// the two sources use different work-id namespaces.
	f.fanza.resolveErr = nil
	f.fanza.ids["河北彩花"] = "1044864"
	f.discover.actorMovies["ZY5eq"] = []domain.Movie{
		{ID: "NEW-001", Code: "NEW-001", ReleaseDate: "2099-01-01"},
		{ID: "NEW-002", Code: "NEW-002", ReleaseDate: "2099-02-01"},
	}
	f.forceDue(t, actor.ID)
	if err := f.service.Check(ctx); err != nil {
		t.Fatalf("the check must keep using JavDB: %v", err)
	}
	if record := f.client.Subscription.GetX(ctx, actor.ID); record.FanzaID != "" {
		t.Fatalf("a JavDB-based subscription must not switch sources: %+v", record)
	}
	if feed, _ := f.service.ActorFeed(ctx, actor.ID, 1, 10); len(feed) != 2 {
		t.Fatalf("the new JavDB work must be tracked once: %#v", feed)
	}
	f.identities["8"] = ActorIdentity{Name: "無名", Names: []string{"無名"}}
	if _, err := f.service.AddActor(ctx, "8"); err != nil {
		t.Fatalf("following must not wait on source resolution: %v", err)
	}
	if err := f.service.Check(ctx); err == nil {
		t.Fatal("without a JavDB id the FANZA failure must surface from the check")
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
