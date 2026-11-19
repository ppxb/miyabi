package actors

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
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

	a1 := db.Actor.Create().SetProvider("javdb").SetSourceID("x1").SetName("七ツ森 りり").SaveX(ctx)
	a2 := db.Actor.Create().SetProvider("duga").SetSourceID("x2").SetName("七ツ森りり").SaveX(ctx)
	db.Movie.Create().SetCode("TEST-001").AddActors(a1, a2).SaveX(ctx)

	svc := New(db, nil, "", nil)
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

	a1 := db.Actor.Create().SetProvider("javdb").SetSourceID("x1").SetName("七ツ森 りり").SaveX(ctx)
	a2 := db.Actor.Create().SetProvider("duga").SetSourceID("x2").SetName("七ツ森りり").SaveX(ctx)
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

	// A second Detail must not duplicate the persisted images.
	if _, err := svc.Detail(ctx, key); err != nil {
		t.Fatal(err)
	}
	if n := db.ActorImage.Query().CountX(ctx); n != 2 {
		t.Fatalf("images duplicated on repeat Detail: %d", n)
	}
}
