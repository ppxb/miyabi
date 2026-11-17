package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/export"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestLocalScanMetadataMode(t *testing.T) {
	cases := []struct {
		name         string
		metadataOnly bool
		wantScrape   int
		wantStatus   movie.ScrapeStatus
	}{
		{"auto completes metadata", false, 1, movie.ScrapeStatusPending},
		{"local keeps local data", true, 0, movie.ScrapeStatusDone},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := t.Context()
			store, err := database.Open(ctx, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })

			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "ABC-123.strm"), []byte("https://example.com/v"), 0o600); err != nil {
				t.Fatal(err)
			}
			queue := tasks.NewService(store.Client, tasks.NewRegistry())
			manager := export.NewManager(export.Config{EmbyDir: root})
			lib := New(store.Client, nil, queue, nil, Options{ExportManager: manager})
			queue.Registry().Register(tasks.NewHandler(tasks.KindScan, lib.Scan, lib.Finished))

			if _, err := lib.StartLocalSourceScanMode(ctx, root, testCase.metadataOnly); err != nil {
				t.Fatal(err)
			}
			job, err := queue.Queue().Claim(ctx, []tasks.Kind{tasks.KindScan})
			if err != nil || job == nil {
				t.Fatalf("claim: %v %v", job, err)
			}
			if err := lib.Scan(ctx, *job); err != nil {
				t.Fatal(err)
			}
			if err := queue.Queue().Finish(ctx, job.ID, nil); err != nil {
				t.Fatal(err)
			}

			film, err := store.Client.Movie.Query().Where(movie.CodeEQ("ABC-123")).Only(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if film.ScrapeStatus != testCase.wantStatus {
				t.Fatalf("scrape status = %v, want %v", film.ScrapeStatus, testCase.wantStatus)
			}
			count := store.Client.Task.Query().Where(task.TypeEQ(tasks.KindScrape.String())).CountX(ctx)
			if count != testCase.wantScrape {
				t.Fatalf("scrape tasks = %d, want %d", count, testCase.wantScrape)
			}
		})
	}
}
