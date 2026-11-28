package scrape

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/nfo"
)

func TestCanonicalizeActorsUsesOneProfileIdentity(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	db := store.Client

	profile := db.ActorProfile.Create().
		SetMatchKey(domain.NormalizeActorName("森日向子")).
		SetName("森日向子").SaveX(ctx)
	first := db.Movie.Create().SetCode("MIKR-096").SaveX(ctx)
	fanza := db.Actor.Create().SetProvider("fanza").SetSourceID("1061586").
		SetName("森日向子").AddMovieIDs(first.ID).SaveX(ctx)
	javdb := db.Actor.Create().SetProvider("javdb").SetSourceID("bkxd").
		SetName("森日向子").AddMovieIDs(first.ID).SaveX(ctx)
	db.Actor.Update().Where(actor.IDIn(fanza.ID, javdb.ID)).SetProfileID(profile.ID).ExecX(ctx)

	doc := nfo.Movie{Code: "MIKR-096", Actors: []nfo.Actor{
		{Provider: "fanza", ID: "1061586", Name: "森日向子"},
		{Provider: "javdb", ID: "bkxd", Name: "森日向子"},
	}}
	original := append([]nfo.Actor(nil), doc.Actors...)
	actors, err := canonicalActors(ctx, db, first.ID, doc.Actors)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc.Actors, original) {
		t.Fatal("canonicalActors mutated its input")
	}
	doc.Actors = actors
	canonical := "miyabi-actor-" + strconv.Itoa(profile.ID)
	if len(doc.Actors) != 1 || doc.Actors[0].MediaServerID != canonical ||
		doc.Actors[0].Name != "森日向子" || doc.Actors[0].Provider != "fanza" ||
		doc.Actors[0].ID != "1061586" {
		t.Fatalf("canonical actors = %+v", doc.Actors)
	}

	// A later movie whose actor spelling differs still gets the same identity.
	second := db.Movie.Create().SetCode("MKMP-749").SaveX(ctx)
	avmoo := db.Actor.Create().SetProvider("avmoo").SetSourceID("njeypjk").
		SetName("森日 向子").AddMovieIDs(second.ID).SaveX(ctx)
	db.Actor.Update().Where(actor.IDEQ(avmoo.ID)).SetProfileID(profile.ID).ExecX(ctx)
	next := nfo.Movie{Code: "MKMP-749", Actors: []nfo.Actor{
		{Provider: "avmoo", ID: "njeypjk", Name: "森日 向子"},
	}}
	next.Actors, err = canonicalActors(ctx, db, second.ID, next.Actors)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Actors) != 1 || next.Actors[0].MediaServerID != canonical ||
		next.Actors[0].Name != "森日向子" {
		t.Fatalf("cross-movie canonical actors = %+v", next.Actors)
	}

	// An actor without a matching source row still joins a profile through an alias.
	if err := store.Client.ActorAlias.Create().
		SetActorID(profile.ID).SetAlias("Kanna Seto").SetNormalized("kannaseto").Exec(ctx); err != nil {
		t.Fatal(err)
	}
	third := db.Movie.Create().SetCode("ABP-901").SaveX(ctx)
	linked := db.Actor.Create().SetProvider("fanza").SetSourceID("f901").
		SetName("森日向子").AddMovieIDs(third.ID).SaveX(ctx)
	db.Actor.Update().Where(actor.IDEQ(linked.ID)).SetProfileID(profile.ID).ExecX(ctx)
	aliased, err := canonicalActors(ctx, db, third.ID, []nfo.Actor{
		{Provider: "unknown", ID: "missing", Name: "Kanna Seto"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(aliased) != 1 || aliased[0].MediaServerID != canonical || aliased[0].Name != "森日向子" {
		t.Fatalf("alias canonical actors = %+v", aliased)
	}
}
