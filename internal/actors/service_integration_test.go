package actors

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/actorprofile"
	"github.com/ppxb/miyabi/internal/ent/subscription"
	"github.com/ppxb/miyabi/internal/gfriends"
)

func openDB(t *testing.T) *ent.Client {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store.Client
}

// link runs LinkProfiles for the given actors the way SaveMovieMetadata does.
func link(t *testing.T, db *ent.Client, actors ...*ent.Actor) {
	t.Helper()
	ids := make([]int, 0, len(actors))
	for _, a := range actors {
		ids = append(ids, a.ID)
	}
	if err := ent.WithTx(t.Context(), db, func(tx *ent.Tx) error { return LinkProfiles(t.Context(), tx, ids) }); err != nil {
		t.Fatal(err)
	}
}

func pidOf(t *testing.T, db *ent.Client, actorID int) int {
	t.Helper()
	r, err := db.Actor.Query().Where(actor.IDEQ(actorID)).
		WithProfile(func(q *ent.ActorProfileQuery) { q.Select(actorprofile.FieldID) }).Only(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if r.Edges.Profile == nil {
		return 0
	}
	return r.Edges.Profile.ID
}

func TestFetchAvatarTriesEveryScrapedRow(t *testing.T) {
	db := openDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/broken":
			http.Error(w, "broken", http.StatusBadGateway)
		case "/good":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("avatar"))
		default:
			http.NotFound(w, request)
		}
	}))
	t.Cleanup(server.Close)

	broken := db.Actor.Create().SetProvider("javdb").SetSourceID("broken").SetName("Fallback Avatar").
		SetAvatar(server.URL + "/broken").SaveX(t.Context())
	good := db.Actor.Create().SetProvider("avmoo").SetSourceID("good").SetName("Fallback Avatar").
		SetAvatar(server.URL + "/good").SaveX(t.Context())
	link(t, db, broken, good)

	svc := New(db, nil, "", server.Client())
	if err := svc.UpdateSettings(t.Context(), []domain.SourceToggle{{ID: "av-league", Enabled: true}, {ID: "gfriends", Enabled: false}}); err != nil {
		t.Fatal(err)
	}
	data, err := svc.FetchAvatar(t.Context(), pidOf(t, db, good.ID))
	if err != nil || string(data) != "avatar" {
		t.Fatalf("FetchAvatar=%q err=%v", data, err)
	}
}

func TestLinkProfilesGroupsSpellingsIntoCatalog(t *testing.T) {
	ctx := t.Context()
	db := openDB(t)
	a1 := db.Actor.Create().SetProvider("javdb").SetSourceID("x1").SetName("七ツ森 りり").SaveX(ctx)
	a2 := db.Actor.Create().SetProvider("duga").SetSourceID("x2").SetName("七ツ森りり").SaveX(ctx)
	lone := db.Actor.Create().SetProvider("javdb").SetSourceID("x3").SetName("無作品").SaveX(ctx)
	db.Movie.Create().SetCode("TEST-001").AddActors(a1, a2).SaveX(ctx)
	link(t, db, a1, a2, lone)
	link(t, db, a1, a2) // already linked: no new profile or alias

	if n := db.ActorProfile.Query().CountX(ctx); n != 2 {
		t.Fatalf("profiles = %d, want 2", n)
	}
	if n := db.ActorAlias.Query().CountX(ctx); n != 3 {
		t.Fatalf("aliases = %d, want 3", n)
	}
	if p1, p2 := pidOf(t, db, a1.ID), pidOf(t, db, a2.ID); p1 == 0 || p1 != p2 {
		t.Fatalf("spellings not on one profile: %d, %d", p1, p2)
	}

	svc := New(db, nil, "", nil)
	page, err := svc.Catalog(ctx, CatalogFilter{Page: 1, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	// The actor without movies stays out of the catalog.
	if page.Total != 1 || len(page.Actors) != 1 {
		t.Fatalf("catalog = %+v", page)
	}
	got := page.Actors[0]
	if got.Name != "七ツ森 りり" || got.MovieCount != 1 || len(got.Sources) != 2 || got.Key != strconv.Itoa(pidOf(t, db, a1.ID)) {
		t.Fatalf("summary = %+v", got)
	}
	for q, want := range map[string]int{"七ツ森": 1, "ﾅﾅﾂ": 0, "りり": 1} {
		page, err := svc.Catalog(ctx, CatalogFilter{Query: q, Page: 1, Limit: 10})
		if err != nil || page.Total != want {
			t.Fatalf("search %q: total %d, err %v", q, page.Total, err)
		}
	}
}

func TestCatalogPagesInSQL(t *testing.T) {
	ctx := t.Context()
	db := openDB(t)
	m := db.Movie.Create().SetCode("PAGE-001").SaveX(ctx)
	var created []*ent.Actor
	for _, name := range []string{"c", "a", "b"} {
		created = append(created, db.Actor.Create().SetProvider("p").SetSourceID(name).SetName(name).AddMovieIDs(m.ID).SaveX(ctx))
	}
	link(t, db, created...)

	svc := New(db, nil, "", nil)
	first, err := svc.Catalog(ctx, CatalogFilter{Page: 1, Limit: 2})
	if err != nil || first.Total != 3 || !first.HasMore || len(first.Actors) != 2 || first.Actors[0].Name != "a" || first.Actors[1].Name != "b" {
		t.Fatalf("page 1 = %+v, %v", first, err)
	}
	second, err := svc.Catalog(ctx, CatalogFilter{Page: 2, Limit: 2})
	if err != nil || second.HasMore || len(second.Actors) != 1 || second.Actors[0].Name != "c" {
		t.Fatalf("page 2 = %+v, %v", second, err)
	}
}

func TestCatalogFiltersFollowing(t *testing.T) {
	ctx := t.Context()
	db := openDB(t)
	m := db.Movie.Create().SetCode("FOLLOW-001").SaveX(ctx)
	followedActor := db.Actor.Create().SetProvider("p").SetSourceID("a").SetName("Followed").AddMovieIDs(m.ID).SaveX(ctx)
	otherActor := db.Actor.Create().SetProvider("p").SetSourceID("b").SetName("Other").AddMovieIDs(m.ID).SaveX(ctx)
	link(t, db, followedActor, otherActor)
	profileID := pidOf(t, db, followedActor.ID)
	db.Subscription.Create().SetKind(subscription.KindActor).SetTargetID(strconv.Itoa(profileID)).SaveX(ctx)

	svc := New(db, nil, "", nil)
	following := true
	page, err := svc.Catalog(ctx, CatalogFilter{Following: &following, Page: 1, Limit: 10})
	if err != nil || page.Total != 1 || len(page.Actors) != 1 || page.Actors[0].Name != "Followed" {
		t.Fatalf("following catalog = %+v, %v", page, err)
	}

	notFollowing := false
	page, err = svc.Catalog(ctx, CatalogFilter{Following: &notFollowing, Page: 1, Limit: 10})
	if err != nil || page.Total != 1 || len(page.Actors) != 1 || page.Actors[0].Name != "Other" {
		t.Fatalf("unfollowed catalog = %+v, %v", page, err)
	}

	page, err = svc.Catalog(ctx, CatalogFilter{Page: 1, Limit: 10})
	if err != nil || page.Total != 2 {
		t.Fatalf("all catalog = %+v, %v", page, err)
	}
}

func TestCatalogFollowingWithoutSubscriptions(t *testing.T) {
	ctx := t.Context()
	db := openDB(t)
	m := db.Movie.Create().SetCode("FOLLOW-EMPTY").SaveX(ctx)
	a := db.Actor.Create().SetProvider("p").SetSourceID("a").SetName("Only").AddMovieIDs(m.ID).SaveX(ctx)
	link(t, db, a)

	following := true
	page, err := New(db, nil, "", nil).Catalog(ctx, CatalogFilter{Following: &following, Page: 1, Limit: 10})
	if err != nil || page.Total != 0 || len(page.Actors) != 0 || page.HasMore {
		t.Fatalf("following catalog = %+v, %v", page, err)
	}
}

func TestCatalogFollowingResolvesLegacySubscriptions(t *testing.T) {
	ctx := t.Context()
	db := openDB(t)
	m := db.Movie.Create().SetCode("LEGACY-001").SaveX(ctx)
	sourceActor := db.Actor.Create().SetProvider("javdb").SetSourceID("legacy-source").SetName("Legacy Source").AddMovieIDs(m.ID).SaveX(ctx)
	fanzaActor := db.Actor.Create().SetProvider("fanza").SetSourceID("1044864").SetName("Legacy Fanza").AddMovieIDs(m.ID).SaveX(ctx)
	link(t, db, sourceActor, fanzaActor)
	db.Subscription.Create().SetKind(subscription.KindActor).SetTargetID("legacy-source").SaveX(ctx)
	db.Subscription.Create().SetKind(subscription.KindActor).SetTargetID("old-fanza-key").SetFanzaID("1044864").SaveX(ctx)

	following := true
	page, err := New(db, nil, "", nil).Catalog(ctx, CatalogFilter{Following: &following, Page: 1, Limit: 10})
	if err != nil || page.Total != 2 {
		t.Fatalf("legacy following catalog = %+v, %v", page, err)
	}
	names := make(map[string]bool, len(page.Actors))
	for _, actor := range page.Actors {
		names[actor.Name] = true
	}
	if !names["Legacy Source"] || !names["Legacy Fanza"] {
		t.Fatalf("legacy follows = %+v", page.Actors)
	}
}

func TestCatalogFollowingPrefersProfileTargetOverSourceCollision(t *testing.T) {
	ctx := t.Context()
	db := openDB(t)
	m := db.Movie.Create().SetCode("COLLIDE-001").SaveX(ctx)
	followedActor := db.Actor.Create().SetProvider("javdb").SetSourceID("followed").SetName("Followed").AddMovieIDs(m.ID).SaveX(ctx)
	link(t, db, followedActor)
	profileID := pidOf(t, db, followedActor.ID)
	// An unrelated provider actor whose id collides with the profile key.
	collision := db.Actor.Create().SetProvider("fanza").SetSourceID(strconv.Itoa(profileID)).SetName("Collision").AddMovieIDs(m.ID).SaveX(ctx)
	link(t, db, collision)
	db.Subscription.Create().SetKind(subscription.KindActor).SetTargetID(strconv.Itoa(profileID)).SaveX(ctx)

	following := true
	page, err := New(db, nil, "", nil).Catalog(ctx, CatalogFilter{Following: &following, Page: 1, Limit: 10})
	if err != nil || page.Total != 1 || len(page.Actors) != 1 || page.Actors[0].Name != "Followed" {
		t.Fatalf("collision follow = %+v, %v", page, err)
	}
}

func TestCatalogFollowingUsesProfileAfterUnmatchedProviderID(t *testing.T) {
	ctx := t.Context()
	db := openDB(t)
	m := db.Movie.Create().SetCode("REMOTE-001").SaveX(ctx)
	actor := db.Actor.Create().SetProvider("javdb").SetSourceID("remote-source").SetName("Remote Follow").AddMovieIDs(m.ID).SaveX(ctx)
	link(t, db, actor)
	profileID := pidOf(t, db, actor.ID)
	// The monitor can persist a remotely resolved id with no local Actor row.
	db.Subscription.Create().
		SetKind(subscription.KindActor).
		SetTargetID(strconv.Itoa(profileID)).
		SetFanzaID("remote-fanza-id").
		SaveX(ctx)

	following := true
	page, err := New(db, nil, "", nil).Catalog(ctx, CatalogFilter{Following: &following, Page: 1, Limit: 10})
	if err != nil || page.Total != 1 || len(page.Actors) != 1 || page.Actors[0].Name != "Remote Follow" {
		t.Fatalf("remote provider follow = %+v, %v", page, err)
	}
}

func TestDetailPersistsAliasesAndImages(t *testing.T) {
	ctx := t.Context()
	db := openDB(t)
	a1 := db.Actor.Create().SetProvider("javdb").SetSourceID("x1").SetName("七ツ森 りり").SaveX(ctx)
	a2 := db.Actor.Create().SetProvider("duga").SetSourceID("x2").SetName("七ツ森りり").SaveX(ctx)
	db.Movie.Create().SetCode("TEST-001").AddActors(a1, a2).SaveX(ctx)
	link(t, db, a1, a2)

	gfDir := t.TempDir()
	tree := `{"Content":{"S":{"七ツ森りり.jpg":"a.jpg","七ツ森 りり.jpg":"b.jpg"}}}`
	if err := os.WriteFile(filepath.Join(gfDir, "gfriends_tree.json"), []byte(tree), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := New(db, gfriends.New(gfDir, nil), "", nil)
	pid := pidOf(t, db, a1.ID)

	detail, err := svc.Detail(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Aliases) != 1 || detail.Aliases[0] != "七ツ森りり" {
		t.Fatalf("aliases must exclude the display name, got %+v", detail.Aliases)
	}
	if len(detail.Names) != 2 || detail.ImageCount != 2 {
		t.Fatalf("names %v, images %d", detail.Names, detail.ImageCount)
	}
	if _, err := svc.Detail(ctx, pid); err != nil {
		t.Fatal(err)
	}
	if n := db.ActorImage.Query().CountX(ctx); n != 2 {
		t.Fatalf("images duplicated on repeat Detail: %d", n)
	}
	if _, err := svc.Detail(ctx, pid+100); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown profile: %v", err)
	}
}

type countingProvider struct {
	calls int
	err   error
}

func (p *countingProvider) Name() string { return "av-league" }

func (p *countingProvider) Lookup(context.Context, string) (*Profile, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	return &Profile{Birthday: "2000-01-01"}, nil
}

// A finished lookup (found or not) is recorded in enriched_at and never
// repeated; a transient error leaves it unset so the next view retries.
func TestDetailEnrichesOnce(t *testing.T) {
	ctx := t.Context()
	db := openDB(t)
	a := db.Actor.Create().SetProvider("javdb").SetSourceID("x1").SetName("瀬戸環奈").SaveX(ctx)
	db.Movie.Create().SetCode("TEST-001").AddActors(a).SaveX(ctx)
	link(t, db, a)
	pid := pidOf(t, db, a.ID)

	provider := &countingProvider{err: errors.New("timeout")}
	svc := New(db, nil, "", nil, provider)
	// gfriends is switched off so the test never reaches its remote index.
	if err := svc.UpdateSettings(ctx, []domain.SourceToggle{{ID: "av-league", Enabled: true}, {ID: "gfriends", Enabled: false}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Detail(ctx, pid); err != nil {
		t.Fatal(err)
	}
	if db.ActorProfile.GetX(ctx, pid).EnrichedAt != nil {
		t.Fatal("transient error marked the profile enriched")
	}
	provider.err = nil
	detail, err := svc.Detail(ctx, pid)
	if err != nil || detail.Birthday != "2000-01-01" {
		t.Fatalf("retry after error: %+v, %v", detail, err)
	}
	if _, err := svc.Detail(ctx, pid); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 2 || db.ActorProfile.GetX(ctx, pid).EnrichedAt == nil {
		t.Fatalf("provider calls = %d, want 2 and enriched_at set", provider.calls)
	}
}

type blockingProvider struct {
	started chan struct{}
	release chan struct{}
}

func (p *blockingProvider) Name() string { return "av-league" }

func (p *blockingProvider) Lookup(context.Context, string) (*Profile, error) {
	p.started <- struct{}{}
	<-p.release
	return &Profile{Birthday: "2000-01-01"}, nil
}

// A viewer leaving mid-lookup must not cancel the shared enrichment attempt.
func TestDetailEnrichmentOutlivesTheRequest(t *testing.T) {
	db := openDB(t)
	a := db.Actor.Create().SetProvider("javdb").SetSourceID("x1").SetName("瀬戸環奈").SaveX(t.Context())
	db.Movie.Create().SetCode("TEST-001").AddActors(a).SaveX(t.Context())
	link(t, db, a)
	pid := pidOf(t, db, a.ID)

	provider := &blockingProvider{started: make(chan struct{}, 1), release: make(chan struct{})}
	svc := New(db, nil, "", nil, provider)
	if err := svc.UpdateSettings(t.Context(), []domain.SourceToggle{{ID: "av-league", Enabled: true}, {ID: "gfriends", Enabled: false}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		<-provider.started
		cancel()
	}()
	_, _ = svc.Detail(ctx, pid) // returns once the viewer leaves, before the lookup ends
	close(provider.release)

	deadline := time.Now().Add(5 * time.Second)
	for db.ActorProfile.GetX(t.Context(), pid).EnrichedAt == nil {
		if time.Now().After(deadline) {
			t.Fatal("enrichment was cancelled with the request")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Merging folds the Latin profile into the CJK one and deletes it; the Latin
// name scraped later for another movie joins the merged profile via its alias.
func TestMergeMovieCrossLanguageAndLaterLatinActor(t *testing.T) {
	ctx := t.Context()
	db := openDB(t)
	m1 := db.Movie.Create().SetCode("MERGE-001").SaveX(ctx)
	ja := db.Actor.Create().SetProvider("fanza").SetSourceID("j1").SetName("瀬戸環奈").AddMovieIDs(m1.ID).SaveX(ctx)
	en := db.Actor.Create().SetProvider("javdatabase").SetSourceID("e1").SetName("Kanna Seto").AddMovieIDs(m1.ID).SaveX(ctx)
	link(t, db, ja, en)
	enProfile := pidOf(t, db, en.ID)

	svc := New(db, nil, "", nil)
	if n, err := svc.MergeMovieCrossLanguage(ctx, m1.ID); err != nil || n != 1 {
		t.Fatalf("merged=%d, err=%v", n, err)
	}
	canonical := pidOf(t, db, ja.ID)
	if pidOf(t, db, en.ID) != canonical {
		t.Fatal("Latin actor not moved to the canonical profile")
	}
	if db.ActorProfile.Query().Where(actorprofile.ID(enProfile)).ExistX(ctx) {
		t.Fatal("merged profile was not deleted")
	}
	if _, err := svc.Detail(ctx, enProfile); !errors.Is(err, ErrNotFound) {
		t.Fatalf("merged profile page: %v", err)
	}
	if n, err := svc.MergeMovieCrossLanguage(ctx, m1.ID); err != nil || n != 0 {
		t.Fatalf("repeat merge=%d, err=%v", n, err)
	}

	m2 := db.Movie.Create().SetCode("MERGE-002").SaveX(ctx)
	later := db.Actor.Create().SetProvider("duga").SetSourceID("e2").SetName("Kanna Seto").AddMovieIDs(m2.ID).SaveX(ctx)
	link(t, db, later)
	if pidOf(t, db, later.ID) != canonical {
		t.Fatal("later Latin actor did not join the merged profile")
	}
	page, err := svc.Catalog(ctx, CatalogFilter{Query: "kanna", Page: 1, Limit: 10})
	if err != nil || len(page.Actors) != 1 || page.Actors[0].MovieCount != 2 || page.Actors[0].Name != "瀬戸環奈" {
		t.Fatalf("catalog = %+v, %v", page, err)
	}
}

func TestMergeMovieCrossLanguageSkipsMultiCast(t *testing.T) {
	ctx := t.Context()
	db := openDB(t)
	m := db.Movie.Create().SetCode("MERGE-002").SaveX(ctx)
	var cast []*ent.Actor
	for _, a := range [][2]string{{"a", "瀬戸環奈"}, {"b", "奥田咲"}, {"c", "Kanna Seto"}, {"d", "Saki Okuda"}} {
		cast = append(cast, db.Actor.Create().SetProvider("p").SetSourceID(a[0]).SetName(a[1]).AddMovieIDs(m.ID).SaveX(ctx))
	}
	link(t, db, cast...)

	svc := New(db, nil, "", nil)
	if n, err := svc.MergeMovieCrossLanguage(ctx, m.ID); err != nil || n != 0 {
		t.Fatalf("merged=%d, err=%v; want 0 for an ambiguous multi-cast", n, err)
	}
	if n := db.ActorProfile.Query().CountX(ctx); n != 4 {
		t.Fatalf("profiles = %d, want 4", n)
	}
}

// A linked actor renamed by a later scrape keeps its profile and the new
// spelling becomes a searchable alias.
func TestLinkProfilesAliasesRenamedLinkedActor(t *testing.T) {
	ctx := t.Context()
	db := openDB(t)
	m := db.Movie.Create().SetCode("RENAME-001").SaveX(ctx)
	a := db.Actor.Create().SetProvider("javdatabase").SetSourceID("e1").SetName("Kanna Seto").AddMovieIDs(m.ID).SaveX(ctx)
	link(t, db, a)
	pid := pidOf(t, db, a.ID)

	a = a.Update().SetName("Seto Kanna").SaveX(ctx)
	link(t, db, a)
	if pidOf(t, db, a.ID) != pid || db.ActorProfile.Query().CountX(ctx) != 1 {
		t.Fatal("rename moved the actor to another profile")
	}
	page, err := New(db, nil, "", nil).Catalog(ctx, CatalogFilter{Query: "seto kanna", Page: 1, Limit: 10})
	if err != nil || page.Total != 1 {
		t.Fatalf("new spelling not searchable: %+v, %v", page, err)
	}
}

func TestIdentityListsSpellingsAndProviderIDs(t *testing.T) {
	ctx := t.Context()
	db := openDB(t)
	a1 := db.Actor.Create().SetProvider("fanza").SetSourceID("1044864").SetName("河北彩花").SaveX(ctx)
	a2 := db.Actor.Create().SetProvider("javdb").SetSourceID("ZY5eq").SetName("河北 彩花").SaveX(ctx)
	db.Movie.Create().SetCode("TEST-001").AddActors(a1, a2).SaveX(ctx)
	link(t, db, a1, a2)
	pid := pidOf(t, db, a1.ID)
	db.ActorAlias.Create().SetActorID(pid).SetAlias("河北彩伽").SetNormalized("河北彩伽").SaveX(ctx)

	identity, err := New(db, nil, "", nil).Identity(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if identity.SourceIDs["fanza"] != "1044864" || identity.SourceIDs["javdb"] != "ZY5eq" {
		t.Fatalf("provider ids: %#v", identity.SourceIDs)
	}
	if identity.Names[0] != identity.Name || !slices.Contains(identity.Names, "河北 彩花") || !slices.Contains(identity.Names, "河北彩伽") {
		t.Fatalf("spellings: %#v", identity)
	}
	if _, err := New(db, nil, "", nil).Identity(ctx, pid+100); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing actor: %v", err)
	}
}
