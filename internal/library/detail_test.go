package library

import (
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/nfo"
)

func TestSavedDetailNeedsNoJavDBAndKeepsSourceIdentities(t *testing.T) {
	lib, _, payload := libraryFixture(t)
	ctx := t.Context()
	doc := nfo.Movie{Code: "ABP-123", Title: "Official title", Summary: "Summary", Zone: domain.ZoneCensored,
		IDs:    []nfo.UniqueID{{Type: "fanza", Value: "abp00123"}},
		Rating: 4.2, RatingSource: "fanza", RatingMax: 5,
		Actors: []nfo.Actor{{Provider: "fanza", ID: "actor", Name: "Actor"}},
		Tags:   []nfo.Tag{{Provider: "fanza", ID: "tag", Name: "Tag"}},
		Studio: nfo.Entity{Provider: "fanza", ID: "maker", Name: "Maker"},
		Images: []domain.ImageCandidate{{Provider: "fanza", URL: "https://image.example/cover.jpg", Role: "cover"},
			{Provider: "fanza", URL: "https://image.example/preview.jpg", Role: "preview"},
			{Provider: "fc2", URL: "https://image.example/fc2-preview.jpg", Role: "preview"}},
	}
	film := lib.database.Movie.Create().SetCode(doc.Code).
		SetPoster("/api/library/artwork/poster").SetFanarts([]string{"/api/library/artwork/full"}).SaveX(ctx)
	file := lib.database.File.Create().SetFileID("detail-video").SetName(doc.Code + ".mp4").SetSize(1024).
		SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).SetMovieID(film.ID).SaveX(ctx)
	jobsBefore := lib.database.Task.Query().CountX(ctx)
	pending, err := lib.Movie(ctx, film.ID)
	if err != nil || pending.Code != doc.Code || pending.ScrapeStatus != movie.ScrapeStatusPending ||
		pending.Actors == nil || pending.Tags == nil || pending.PreviewImages == nil {
		t.Fatalf("pending detail = %+v, %v", pending, err)
	}
	if _, err := lib.Preview(ctx, film.ID, 0); !domain.IsKind(err, domain.KindNotFound) {
		t.Fatalf("pending preview: %v", err)
	}
	film.Update().SetMetadata(&doc).SetScrapeStatus(movie.ScrapeStatusDone).ExecX(ctx)
	detail, err := lib.Movie(ctx, film.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.LibraryID != film.ID || detail.ID != "" || detail.Title != doc.Title || detail.Summary != doc.Summary || detail.Zone != doc.Zone {
		t.Fatalf("wrong saved detail: %+v", detail)
	}
	if detail.RatingSource != "fanza" || detail.RatingMax != 5 || detail.Actors[0].Provider != "fanza" || detail.Maker.Provider != "fanza" {
		t.Fatalf("source identity lost: %+v", detail)
	}
	if detail.Cover != "/api/library/artwork/full" || len(detail.PreviewImages) != 2 || !strings.Contains(detail.PreviewImages[0].Original, "/previews/0?v=") {
		t.Fatalf("wrong artwork projection: %+v", detail)
	}
	for i, want := range doc.Images[1:] {
		candidate, err := lib.Preview(ctx, film.ID, i)
		if err != nil || candidate != want {
			t.Fatalf("preview lost source: %+v %v", candidate, err)
		}
	}
	if _, err := lib.Preview(ctx, film.ID, 2); !domain.IsKind(err, domain.KindNotFound) {
		t.Fatalf("missing preview: %v", err)
	}
	if lib.database.Task.Query().CountX(ctx) != jobsBefore {
		t.Fatal("opening details started work")
	}
	file.Update().SetAccountID("other-account").ExecX(ctx)
	if _, err := lib.Movie(ctx, film.ID); !domain.IsKind(err, domain.KindNotFound) {
		t.Fatalf("detail escaped library scope: %v", err)
	}
	if _, err := lib.Preview(ctx, film.ID, 0); !domain.IsKind(err, domain.KindNotFound) {
		t.Fatalf("preview escaped library scope: %v", err)
	}
}
