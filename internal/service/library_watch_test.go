package service

import (
	"errors"
	"testing"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/nfo"
)

func TestMarkWatchedIsLocalAndIdempotent(t *testing.T) {
	library, queued, payload := libraryFixture(t)
	ctx := t.Context()
	if err := library.indexScanPage(ctx, queued.ID, "first", "/Movies",
		[]scanVideo{fixtureVideo("video", "ABP-001.mp4")}, &payload); err != nil {
		t.Fatal(err)
	}
	film := library.database.Movie.Query().OnlyX(ctx)
	if film.Watched {
		t.Fatal("new movies must start unwatched")
	}
	before := library.tasks.Revisions()
	// The fixture has no 115 client; recording an open must work locally.
	if err := library.MarkWatched(ctx, film.ID); err != nil {
		t.Fatal(err)
	}
	updated := library.database.Movie.GetX(ctx, film.ID)
	after := library.tasks.Revisions()
	if !updated.Watched || after.Library != before.Library+1 || after.Offline != before.Offline {
		t.Fatalf("watch change was not persisted and published once: movie=%+v before=%+v after=%+v", updated, before, after)
	}
	page, err := library.Movies(ctx, 1, 24)
	if err != nil || len(page.Movies) != 1 || !page.Movies[0].Watched {
		t.Fatalf("library did not return the saved watch state: %+v, %v", page, err)
	}
	if err := library.MarkWatched(ctx, film.ID); err != nil {
		t.Fatal(err)
	}
	repeated := library.database.Movie.GetX(ctx, film.ID)
	if !repeated.UpdatedAt.Equal(updated.UpdatedAt) || library.tasks.Revisions() != after {
		t.Fatal("opening an already watched movie wrote or broadcast another change")
	}
}

func TestMarkWatchedRequiresAMovieInTheMountedSource(t *testing.T) {
	library, _, payload := libraryFixture(t)
	ctx := t.Context()
	for _, scenario := range []struct{ code, account, root string }{
		{"ABP-001", "other-account", payload.Source.Directory.ID},
		{"ABP-002", payload.Source.AccountID, "other-root"},
		{"ABP-003", "", ""},
	} {
		film := library.database.Movie.Create().SetCode(scenario.code).SaveX(ctx)
		if scenario.account != "" {
			library.database.File.Create().SetFileID(scenario.code).SetName(scenario.code + ".mp4").SetSize(1 << 30).
				SetAccountID(scenario.account).SetRootID(scenario.root).SetMovie(film).ExecX(ctx)
		}
		before := library.tasks.Revisions()
		if err := library.MarkWatched(ctx, film.ID); !ent.IsNotFound(err) {
			t.Fatalf("movie %s outside the mounted source was accepted: %v", film.Code, err)
		}
		if library.database.Movie.GetX(ctx, film.ID).Watched || library.tasks.Revisions() != before {
			t.Fatal("rejected watch request changed movie state")
		}
	}
	if err := library.MarkWatched(ctx, 99999); !ent.IsNotFound(err) {
		t.Fatalf("missing movie was accepted: %v", err)
	}
	library.database.Setting.Delete().ExecX(ctx)
	if err := library.MarkWatched(ctx, 1); !errors.Is(err, ErrMediaDirectoryRequired) {
		t.Fatalf("unmounted library was accepted: %v", err)
	}
}

func TestWatchedStateSurvivesScrapingDownloadIndexingAndRescan(t *testing.T) {
	library, queued, payload := libraryFixture(t)
	ctx := t.Context()
	videos := []scanVideo{fixtureVideo("video", "ABP-001.mp4")}
	if err := library.indexScanPage(ctx, queued.ID, "first", "/Movies", videos, &payload); err != nil {
		t.Fatal(err)
	}
	film := library.database.Movie.Query().OnlyX(ctx)
	if err := library.MarkWatched(ctx, film.ID); err != nil {
		t.Fatal(err)
	}
	doc := nfo.Movie{Code: film.Code, Title: "Updated title",
		IDs: []nfo.UniqueID{{Type: "javdb", Default: true, Value: "catalogue-id"}}}
	if err := ent.WithTx(ctx, library.database, func(tx *ent.Tx) error {
		if err := saveMovieMetadata(ctx, tx, film.ID, doc); err != nil {
			return err
		}
		_, err := indexDownloadedMovie(ctx, tx, scanPayload{Code: film.Code, JavDBID: doc.JavDBID()})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := library.indexScanPage(ctx, queued.ID, "rescan", "/Movies", videos, &payload); err != nil {
		t.Fatal(err)
	}
	got := library.database.Movie.GetX(ctx, film.ID)
	if !got.Watched || got.Title != doc.Title || got.ScrapeStatus != movie.ScrapeStatusPending {
		t.Fatalf("metadata/index updates lost watch state: %+v", got)
	}
}
