package playback

import (
	"context"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
)

func TestWatchHistorySaveResumeAndClear(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	film := store.Client.Movie.Create().SetCode("ABP-001").SetTitle("Title").SaveX(ctx)
	store.Client.File.Create().SetFileID("f1").SetName("ABP-001.mp4").SetSize(1024).
		SetAccountID("100").SetRootID("10").SetMovieID(film.ID).ExecX(ctx)

	service := &Service{database: store.Client}
	if resume, err := service.Resume(ctx, film.ID); err != nil || resume != nil {
		t.Fatalf("initial resume = %+v, %v", resume, err)
	}
	if err := service.SaveProgress(ctx, film.ID, "f1", 120, 3600, &domain.PlaybackSettings{Rate: 2, Quality: "1080p"}); err != nil {
		t.Fatal(err)
	}
	resume, err := service.Resume(ctx, film.ID)
	if err != nil || resume == nil || resume.FileID != "f1" || resume.Position != 120 || resume.Duration != 3600 ||
		resume.Settings == nil || resume.Settings.Rate != 2 || resume.Settings.Quality != "1080p" {
		t.Fatalf("resume = %+v, %v", resume, err)
	}
	// Position is clamped to duration; a partial settings update (rate only)
	// must not clear the previously saved rendition.
	if err := service.SaveProgress(ctx, film.ID, "f1", 4000, 3600, &domain.PlaybackSettings{Rate: 1}); err != nil {
		t.Fatal(err)
	}
	resume, _ = service.Resume(ctx, film.ID)
	if resume == nil || resume.Position != 3600 {
		t.Fatalf("clamped resume = %+v", resume)
	}
	if resume.Settings == nil || resume.Settings.Rate != 1 || resume.Settings.Quality != "1080p" {
		t.Fatalf("partial update should merge, got %+v", resume.Settings)
	}
	// A nil settings save keeps the stored preferences untouched.
	if err := service.SaveProgress(ctx, film.ID, "f1", 4000, 3600, nil); err != nil {
		t.Fatal(err)
	}
	resume, _ = service.Resume(ctx, film.ID)
	if resume == nil || resume.Settings == nil || resume.Settings.Rate != 1 || resume.Settings.Quality != "1080p" {
		t.Fatalf("nil settings should preserve, got %+v", resume.Settings)
	}

	page, err := service.History(ctx, 1)
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].MovieID != film.ID {
		t.Fatalf("history = %+v, %v", page, err)
	}
	if page.Items[0].Title != "Title" || page.Items[0].Code != "ABP-001" {
		t.Fatalf("history item = %+v", page.Items[0])
	}

	removed, err := service.ClearHistory(ctx)
	if err != nil || removed != 1 {
		t.Fatalf("clear = %d, %v", removed, err)
	}
	if page, _ := service.History(ctx, 1); page.Total != 0 {
		t.Fatalf("history not cleared: %+v", page)
	}
}
