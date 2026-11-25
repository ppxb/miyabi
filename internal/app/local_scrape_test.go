package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

// A local source in auto-complete mode scrapes without any 115 session, lands
// in the library and is exported to Emby with .strm files streaming from disk.
func TestLocalSourceAutoCompleteScrapesAndExports(t *testing.T) {
	f := newPipelineFixture(t)
	ctx := t.Context()
	f.addCatalogueMovie(fixtureDetail())
	root := t.TempDir()
	for name, body := range map[string]string{"ABP-123.mp4": "video", "XYZ-999.mp4": "other"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := f.library.StartLocalSourceScanMode(ctx, root, false); err != nil {
		t.Fatal(err)
	}
	f.runQueue(t)

	film := f.store.Client.Movie.Query().Where(movie.CodeEQ("ABP-123")).OnlyX(ctx)
	if film.ScrapeStatus != movie.ScrapeStatusDone || film.Metadata == nil || film.Metadata.Title != "Localized title" || film.Poster == nil {
		t.Fatalf("local movie not completed: status=%v metadata=%+v poster=%v", film.ScrapeStatus, film.Metadata, film.Poster)
	}
	// Metadata the providers cannot find fails the movie instead of leaving it pending.
	missing := f.store.Client.Movie.Query().Where(movie.CodeEQ("XYZ-999")).OnlyX(ctx)
	if missing.ScrapeStatus != movie.ScrapeStatusFailed {
		t.Fatalf("unresolved local movie status = %v, want failed", missing.ScrapeStatus)
	}
	if failed := f.store.Client.Task.Query().Where(task.TypeEQ(tasks.KindScrape.String()), task.StatusEQ(task.StatusFailed)).CountX(ctx); failed != 1 {
		t.Fatalf("failed scrape tasks = %d, want 1", failed)
	}

	videoRow := f.store.Client.File.Query().Where(file.MovieIDEQ(film.ID)).OnlyX(ctx)
	movieDir := filepath.Join(f.embyDir, "ABP", "ABP-123")
	strm, err := os.ReadFile(filepath.Join(movieDir, "ABP-123.strm"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(videoRow.FileID, domain.LocalFileIDPrefix) || !strings.Contains(string(strm), "/api/strm/play/"+videoRow.FileID) {
		t.Fatalf("strm %q does not stream local file %q", strm, videoRow.FileID)
	}
	for _, name := range []string{"ABP-123.nfo", "poster.jpg", "fanart.jpg"} {
		if _, err := os.Stat(filepath.Join(movieDir, name)); err != nil {
			t.Fatal(err)
		}
	}

	// Importing the Emby directory reads that .strm back; the video row must
	// keep pointing at the video on disk.
	if _, err := f.library.StartLocalSourceScanMode(ctx, f.embyDir, true); err != nil {
		t.Fatal(err)
	}
	f.runQueue(t)
	videoRow = f.store.Client.File.GetX(ctx, videoRow.ID)
	if !strings.EqualFold(filepath.Base(videoRow.Path), "ABP-123.mp4") {
		t.Fatalf("Emby import moved the video row onto %q", videoRow.Path)
	}
	if count := f.store.Client.File.Query().Where(file.MovieIDEQ(film.ID)).CountX(ctx); count != 1 {
		t.Fatalf("movie files after Emby import = %d, want 1", count)
	}
}

// A local source inside the Emby directory is already Emby's library: completing
// its metadata must not rewrite the user's .strm into a link to itself.
func TestLocalSourceInsideEmbyDirectoryIsNotReexported(t *testing.T) {
	f := newPipelineFixture(t)
	ctx := t.Context()
	f.addCatalogueMovie(fixtureDetail())
	root := filepath.Join(f.embyDir, "ABP", "ABP-123")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	strmPath := filepath.Join(root, "ABP-123.strm")
	const original = "https://example.com/video.mp4\n"
	if err := os.WriteFile(strmPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := f.library.StartLocalSourceScanMode(ctx, root, false); err != nil {
		t.Fatal(err)
	}
	f.runQueue(t)

	film := f.store.Client.Movie.Query().Where(movie.CodeEQ("ABP-123")).OnlyX(ctx)
	if film.ScrapeStatus != movie.ScrapeStatusDone {
		t.Fatalf("status = %v, want done", film.ScrapeStatus)
	}
	if body, err := os.ReadFile(strmPath); err != nil || string(body) != original {
		t.Fatalf("user .strm rewritten: %q, %v", body, err)
	}
	if _, err := os.Stat(filepath.Join(root, "ABP-123.nfo")); !os.IsNotExist(err) {
		t.Fatalf("exported into the source directory: %v", err)
	}
}
