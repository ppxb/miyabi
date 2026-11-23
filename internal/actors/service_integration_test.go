package actors

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/actorprofile"
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
	page, err := svc.Catalog(ctx, "", 1, 10)
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
		page, err := svc.Catalog(ctx, q, 1, 10)
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
	first, err := svc.Catalog(ctx, "", 1, 2)
	if err != nil || first.Total != 3 || !first.HasMore || len(first.Actors) != 2 || first.Actors[0].Name != "a" || first.Actors[1].Name != "b" {
		t.Fatalf("page 1 = %+v, %v", first, err)
	}
	second, err := svc.Catalog(ctx, "", 2, 2)
	if err != nil || second.HasMore || len(second.Actors) != 1 || second.Actors[0].Name != "c" {
		t.Fatalf("page 2 = %+v, %v", second, err)
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
	page, err := svc.Catalog(ctx, "kanna", 1, 10)
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
	page, err := New(db, nil, "", nil).Catalog(ctx, "seto kanna", 1, 10)
	if err != nil || page.Total != 1 {
		t.Fatalf("new spelling not searchable: %+v, %v", page, err)
	}
}
