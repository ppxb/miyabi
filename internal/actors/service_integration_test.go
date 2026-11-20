package actors

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/actorprofile"
	"github.com/ppxb/miyabi/internal/gfriends"
)

func TestEnsureProfilesAndCatalog(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db := store.Client

	a1 := db.Actor.Create().SetProvider("javdb").SetSourceID("x1").SetName("七ツ森 りり").SetNameKey(domain.ActorNameKey("七ツ森 りり")).SaveX(ctx)
	a2 := db.Actor.Create().SetProvider("duga").SetSourceID("x2").SetName("七ツ森りり").SetNameKey(domain.ActorNameKey("七ツ森りり")).SaveX(ctx)
	db.Movie.Create().SetCode("TEST-001").AddActors(a1, a2).SaveX(ctx)

	svc := New(db, nil, "", nil)
	// Simulate a profile created earlier (e.g. by opening the detail page).
	if _, err := svc.ensureProfile(ctx, normalizeName("七ツ森 りり"), "七ツ森 りり", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnsureProfiles(ctx); err != nil {
		t.Fatal(err)
	}

	if n := db.ActorProfile.Query().CountX(ctx); n != 1 {
		t.Fatalf("expected 1 profile, got %d", n)
	}
	if n := db.ActorAlias.Query().CountX(ctx); n != 2 {
		t.Fatalf("expected 2 aliases, got %d", n)
	}
	if n := db.Actor.Query().Where(actor.HasProfile()).CountX(ctx); n != 2 {
		t.Fatalf("expected both source actors linked to profile, got %d", n)
	}
	if n := db.ActorProfile.Query().Where(actorprofile.HasSources()).CountX(ctx); n != 1 {
		t.Fatalf("expected profile with sources edge, got %d", n)
	}

	page, err := svc.Catalog(ctx, "", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Actors) != 1 {
		t.Fatalf("expected one canonical actor, got %+v", page)
	}
	if page.Actors[0].MovieCount != 1 {
		t.Fatalf("expected movie_count 1, got %d", page.Actors[0].MovieCount)
	}
	if page.Actors[0].Name != "七ツ森 りり" {
		t.Fatalf("unexpected display name %q", page.Actors[0].Name)
	}
}

func TestDetailPersistsAliasesAndImages(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db := store.Client

	a1 := db.Actor.Create().SetProvider("javdb").SetSourceID("x1").SetName("七ツ森 りり").SetNameKey(domain.ActorNameKey("七ツ森 りり")).SaveX(ctx)
	a2 := db.Actor.Create().SetProvider("duga").SetSourceID("x2").SetName("七ツ森りり").SetNameKey(domain.ActorNameKey("七ツ森りり")).SaveX(ctx)
	db.Movie.Create().SetCode("TEST-001").AddActors(a1, a2).SaveX(ctx)

	gfDir := t.TempDir()
	page := `{"Content":{"S":{"七ツ森りり.jpg":"a.jpg","七ツ森 りり.jpg":"b.jpg"}}}`
	if err := os.WriteFile(filepath.Join(gfDir, "gfriends_tree.json"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := New(db, gfriends.New(gfDir, nil), "", nil)
	if err := svc.EnsureProfiles(ctx); err != nil {
		t.Fatal(err)
	}
	catalog, err := svc.Catalog(ctx, "", 1, 10)
	if err != nil || len(catalog.Actors) != 1 {
		t.Fatalf("catalog: %+v %v", catalog, err)
	}
	key := catalog.Actors[0].Key

	detail, err := svc.Detail(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Aliases) != 1 || detail.Aliases[0] != "七ツ森りり" {
		t.Fatalf("aliases must exclude the display name, got %+v", detail.Aliases)
	}
	if detail.ImageCount != 2 {
		t.Fatalf("expected 2 images, got %d", detail.ImageCount)
	}
	if n := db.ActorImage.Query().CountX(ctx); n != 2 {
		t.Fatalf("expected 2 persisted images, got %d", n)
	}
	if p1, p2 := pidOf(t, db, a1.ID), pidOf(t, db, a2.ID); p1 == 0 || p1 != p2 {
		t.Fatalf("detail must link all group actors to one profile: %d, %d", p1, p2)
	}

	// A second Detail must not duplicate the persisted images.
	if _, err := svc.Detail(ctx, key); err != nil {
		t.Fatal(err)
	}
	if n := db.ActorImage.Query().CountX(ctx); n != 2 {
		t.Fatalf("images duplicated on repeat Detail: %d", n)
	}
}

func pidOf(t *testing.T, db *ent.Client, actorID int) int {
	t.Helper()
	r, err := db.Actor.Query().Where(actor.IDEQ(actorID)).
		WithProfile(func(q *ent.ActorProfileQuery) { q.Select(actorprofile.FieldID) }).Only(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.Edges.Profile == nil {
		return 0
	}
	return r.Edges.Profile.ID
}

func TestMergeMovieCrossLanguage(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db := store.Client
	m := db.Movie.Create().SetCode("MERGE-001").SaveX(ctx)
	ja := db.Actor.Create().SetProvider("fanza").SetSourceID("j1").SetName("瀬戸環奈").AddMovieIDs(m.ID).SaveX(ctx)
	en := db.Actor.Create().SetProvider("javdatabase").SetSourceID("e1").SetName("Kanna Seto").AddMovieIDs(m.ID).SaveX(ctx)

	svc := New(db, nil, "", nil)
	n, err := svc.MergeMovieCrossLanguage(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("merged=%d, want 1", n)
	}
	jp, ep := pidOf(t, db, ja.ID), pidOf(t, db, en.ID)
	if jp == 0 || jp != ep {
		t.Fatalf("profiles not merged: ja=%d en=%d", jp, ep)
	}
	page, err := svc.Catalog(ctx, "", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Actors) != 1 {
		t.Fatalf("catalog actors=%d, want 1 (merged)", len(page.Actors))
	}
}

func TestMergeMovieCrossLanguageSkipsMultiCast(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db := store.Client
	m := db.Movie.Create().SetCode("MERGE-002").SaveX(ctx)
	db.Actor.Create().SetProvider("fanza").SetSourceID("a").SetName("瀬戸環奈").AddMovieIDs(m.ID).SaveX(ctx)
	db.Actor.Create().SetProvider("fanza").SetSourceID("b").SetName("奥田咲").AddMovieIDs(m.ID).SaveX(ctx)
	db.Actor.Create().SetProvider("javdatabase").SetSourceID("c").SetName("Kanna Seto").AddMovieIDs(m.ID).SaveX(ctx)
	db.Actor.Create().SetProvider("javdatabase").SetSourceID("d").SetName("Saki Okuda").AddMovieIDs(m.ID).SaveX(ctx)

	svc := New(db, nil, "", nil)
	n, err := svc.MergeMovieCrossLanguage(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("merged=%d, want 0 (ambiguous multi-cast)", n)
	}
}

func TestFindNamesByIndexedKey(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db := store.Client

	key := domain.ActorNameKey("七ツ森 りり")
	db.Actor.Create().SetProvider("javdb").SetSourceID("x1").SetName("七ツ森 りり").SetNameKey(key).SaveX(ctx)
	db.Actor.Create().SetProvider("duga").SetSourceID("x2").SetName("七ツ森りり").SetNameKey(domain.ActorNameKey("七ツ森りり")).SaveX(ctx)

	names, err := New(db, nil, "", nil).findNames(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "七ツ森 りり" || names[1] != "七ツ森りり" {
		t.Fatalf("findNames=%v, want both normalized spellings", names)
	}

	if _, err := New(db, nil, "", nil).findNames(ctx, "missing"); err != ErrNotFound {
		t.Fatalf("findNames(missing) err=%v, want ErrNotFound", err)
	}
}

func TestEnsureMovieProfilesLinksNewActorToExistingProfile(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db := store.Client
	svc := New(db, nil, "", nil)

	// An existing person already has a canonical profile (e.g. from a detail view).
	pid, err := svc.ensureProfile(ctx, normalizeName("七ツ森 りり"), "七ツ森 りり", "", "")
	if err != nil {
		t.Fatal(err)
	}
	known := db.Actor.Create().SetProvider("javdb").SetSourceID("x1").SetName("七ツ森 りり").
		SetNameKey(domain.ActorNameKey("七ツ森 りり")).SetProfileID(pid).SaveX(ctx)
	movie := db.Movie.Create().SetCode("RECON-001").AddActors(known).SaveX(ctx)

	// A newly scraped provider row for the same person, plus an unrelated actor.
	fresh := db.Actor.Create().SetProvider("duga").SetSourceID("x2").SetName("七ツ森りり").
		SetNameKey(domain.ActorNameKey("七ツ森りり")).AddMovieIDs(movie.ID).SaveX(ctx)
	other := db.Actor.Create().SetProvider("duga").SetSourceID("y1").SetName("無関係 太郎").
		SetNameKey(domain.ActorNameKey("無関係 太郎")).AddMovieIDs(movie.ID).SaveX(ctx)

	if err := svc.EnsureMovieProfiles(ctx, movie.ID); err != nil {
		t.Fatal(err)
	}

	if n := db.Actor.Query().Where(actor.HasProfileWith(actorprofile.ID(pid))).CountX(ctx); n != 2 {
		t.Fatalf("linked actors=%d, want 2 (known + fresh)", n)
	}
	if n := db.Actor.Query().Where(actor.IDEQ(other.ID), actor.HasProfile()).CountX(ctx); n != 0 {
		t.Fatalf("unrelated actor should stay profile-less, got %d", n)
	}
	if n := db.ActorProfile.Query().CountX(ctx); n != 1 {
		t.Fatalf("profiles=%d, want 1 (must not create new profiles)", n)
	}
	freshActor := db.Actor.GetX(ctx, fresh.ID)
	if fid, err := freshActor.QueryProfile().OnlyID(ctx); err != nil || fid != pid {
		t.Fatalf("fresh actor profile=%d,%v want %d", fid, err, pid)
	}
}
