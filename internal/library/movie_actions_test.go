package library

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/library/scan"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

func movieActionFixture(t *testing.T) (*libraryTestService, *ent.Movie, domain.ScanPayload) {
	t.Helper()
	lib, queued, payload := libraryFixture(t)
	if err := indexScanPage(t.Context(), lib, queued.ID, "initial", "/Movies", []scan.Video{fixtureVideo("101", "ABP-001.mp4")}, &payload); err != nil {
		t.Fatal(err)
	}
	lib.database.Task.UpdateOneID(queued.ID).SetStatus(task.StatusDone).ExecX(t.Context())
	record := lib.database.Movie.Query().OnlyX(t.Context())
	record = record.Update().SetScrapeStatus(movie.ScrapeStatusDone).SetJavdbID("old-id").SetTitle("old title").
		SetCover("old-cover").SetPoster("old-poster").SetFanarts([]string{"old-fanart"}).
		SetMetadata(&nfo.Movie{Code: record.Code, Title: "old title"}).
		SetMetadataSnapshot(&domain.MetadataSnapshot{Code: record.Code, AccountID: payload.Source.AccountID, DirectoryID: payload.Source.Directory.ID}).SaveX(t.Context())
	return lib, record, payload
}

func TestMovieRescrapeIsFreshScopedAndDeduplicated(t *testing.T) {
	lib, record, source := movieActionFixture(t)
	other := lib.database.Movie.Create().SetCode("IPX-002").SetScrapeStatus(movie.ScrapeStatusDone).SaveX(t.Context())
	var wg sync.WaitGroup
	results := make(chan domain.TaskInfo, 4)
	for range 4 {
		wg.Go(func() {
			info, err := lib.RescrapeMovie(t.Context(), record.ID, "")
			if err != nil {
				t.Error(err)
				return
			}
			results <- info
		})
	}
	wg.Wait()
	close(results)
	var id int
	for info := range results {
		if id != 0 && info.ID != id {
			t.Fatalf("duplicate workflow: %d != %d", info.ID, id)
		}
		id = info.ID
		if info.MovieID != record.ID || info.Code != record.Code || !info.Rebuild || info.Status != "queued" || info.Scan.MetadataTotal != 1 || info.Source != source.Source {
			t.Fatalf("wrong single-movie workflow: %+v", info)
		}
	}
	child := lib.database.Task.Query().Where(task.TypeEQ("scrape")).OnlyX(t.Context())
	input, err := tasks.DecodePayload[scrape.Payload](child.Payload)
	if err != nil || input.MovieID != record.ID || !input.Rebuild || input.MetadataReady || input.Artwork != nil || input.JavDBID != "old-id" {
		t.Fatalf("not a fresh JavDB-aware job: %+v, %v", input, err)
	}
	if lib.database.Movie.GetX(t.Context(), other.ID).ScrapeStatus != movie.ScrapeStatusDone {
		t.Fatal("rescrape modified a different movie")
	}
	if _, err := lib.RescrapeMovie(t.Context(), record.ID, "IPX-123"); !domain.IsKind(err, domain.KindConflict) {
		t.Fatalf("correction while scraping: %v", err)
	}
}

func TestMovieCorrectionValidatesBeforeChangingIdentity(t *testing.T) {
	lib, record, _ := movieActionFixture(t)
	lib.database.Movie.Create().SetCode("IPX-123").ExecX(t.Context())
	for _, code := range []string{"../movie", "https://example.com/ABP-123", "IPX-123"} {
		if _, err := lib.RescrapeMovie(t.Context(), record.ID, code); err == nil {
			t.Fatalf("accepted invalid or duplicate code %q", code)
		}
	}
	got := lib.database.Movie.GetX(t.Context(), record.ID)
	if got.Code != record.Code || got.ManualCode != "" || got.ScrapeStatus != movie.ScrapeStatusDone || domain.ValueOrZero(got.JavdbID) != "old-id" {
		t.Fatalf("rejected correction changed movie: %+v", got)
	}
	if _, err := lib.RescrapeMovie(t.Context(), 999, ""); !domain.IsKind(err, domain.KindNotFound) {
		t.Fatalf("accepted movie outside current source: %v", err)
	}
}

func TestManualCorrectionSurvivesRebuildAndStaleTaskFailure(t *testing.T) {
	lib, record, source := movieActionFixture(t)
	ctx := t.Context()
	info, err := lib.RescrapeMovie(ctx, record.ID, "ipx123")
	if err != nil {
		t.Fatal(err)
	}
	got := lib.database.Movie.GetX(ctx, record.ID)
	if info.Code != "IPX-123" || got.Code != info.Code || got.ManualCode != info.Code || got.JavdbID != nil || got.Title != "" || got.Cover != nil || got.Poster != nil || len(got.Fanarts) != 0 || got.MetadataSnapshot.Code != "ABP-001" {
		t.Fatalf("wrong correction: %+v", got)
	}
	file := lib.database.File.Query().OnlyX(ctx)
	if file.Name != "ABP-001.mp4" || domain.ValueOrZero(file.MovieID) != record.ID {
		t.Fatal("correction renamed or detached the video")
	}
	child := lib.database.Task.Query().Where(task.TypeEQ("scrape")).OnlyX(ctx)
	if err := lib.tasks.Queue().Finish(ctx, child.ID, errors.New("source failed")); err != nil {
		t.Fatal(err)
	}
	if lib.database.Movie.GetX(ctx, record.ID).ScrapeStatus != movie.ScrapeStatusFailed {
		t.Fatal("failed refresh did not mark the movie failed")
	}
	oldBody, _ := tasks.EncodePayload(scrape.MetadataPayload{Rebuild: true, Source: source.Source, MovieID: record.ID, Code: "ABP-001"})
	old := lib.database.Task.Create().SetType("scrape").SetPayload(oldBody).SaveX(ctx)
	lib.database.Movie.UpdateOneID(record.ID).SetScrapeStatus(movie.ScrapeStatusDone).ExecX(ctx)
	if err := lib.tasks.Queue().Finish(ctx, old.ID, errors.New("stale failure")); err != nil {
		t.Fatal(err)
	}
	if lib.database.Movie.GetX(ctx, record.ID).ScrapeStatus != movie.ScrapeStatusDone {
		t.Fatal("stale task overwrote the corrected movie status")
	}
	client := stubOf(t, lib.drive)
	client.list = func(context.Context, string, string, int, int) (pan.FilePage, error) {
		return pan.FilePage{Total: 2, Path: []pan.Directory{{ID: "10", Name: "Movies"}}, Files: []pan.File{
			{ID: "101", ParentID: "10", Name: "ABP-001-renamed.mp4", Size: 1 << 30},
			{ID: "nfo", ParentID: "10", Name: "ABP-001.nfo", PickCode: "old-nfo"},
		}}, nil
	}
	client.readMetadata = func(context.Context, string, string, int64) ([]byte, error) {
		t.Error("manual correction read a stale remote NFO")
		return nil, errors.New("stale NFO")
	}
	queued, err := lib.StartRebuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parent := lib.database.Task.GetX(ctx, queued.ID)
	if err := lib.Scan(ctx, tasks.Job{ID: parent.ID, Payload: parent.Payload}); err != nil {
		t.Fatal(err)
	}
	if lib.database.Movie.Query().CountX(ctx) != 1 || lib.database.Movie.GetX(ctx, record.ID).Code != "IPX-123" {
		t.Fatal("rebuild restored the filename's incorrect identity")
	}
	child = lib.database.Task.Query().Where(task.TypeEQ("scrape"), task.StatusEQ(task.StatusQueued)).OnlyX(ctx)
	input, err := tasks.DecodePayload[scrape.MetadataPayload](child.Payload)
	if err != nil || input.Code != "IPX-123" || input.ManualCode != "IPX-123" || input.JavDBID != "" {
		t.Fatalf("rebuild lost manual identity: %+v %v", input, err)
	}
}
