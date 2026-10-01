package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/export"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestRebuildReplacesSavedMetadataAndPublishesNewPreviews(t *testing.T) {
	for _, tc := range []struct{ code, provider string }{
		{"EBWH-367", "fanza"}, {"IPZZ-960", "fanza"},
		{"HEYZO-3731", "heyzo"}, {"042126_100", "pacopacomama"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			f := newPipelineFixture(t)
			ctx := t.Context()
			official := &workflowSource{id: tc.provider, body: f.catalogue.cover}
			catalogue := &workflowSource{id: "javdb", body: f.catalogue.cover}
			if tc.provider == "fanza" {
				catalogue.fetchError = metadata.ErrNotFound
			} else {
				official.fetchError = metadata.ErrNotFound
			}
			meta, err := metadata.New(ctx, f.store.Client, official, catalogue)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(meta.Close)
			f.scrape = scrape.New(f.store.Client, f.driveService, meta, f.images, f.tasks, scrape.Dependencies{
				ExportManager: export.NewManager(export.Config{EmbyDir: f.embyDir, PublicURL: "http://127.0.0.1:8080"}),
			})
			t.Cleanup(f.scrape.Close)
			f.tasks.Registry().Register(tasks.NewHandler(tasks.KindScrape, f.scrape.Scrape, f.scrape.Finished).WithRetry(domain.RetryDelay))
			f.drive.addFile("101", "10", tc.code+".mp4", 2<<30, []byte("video"))
			if _, err := f.library.StartScan(ctx); err != nil {
				t.Fatal(err)
			}
			f.runQueue(t)
			before := f.store.Client.Movie.Query().OnlyX(ctx)
			if before.ScrapeStatus != movie.ScrapeStatusDone || len(before.Metadata.Previews) != 0 {
				t.Fatal("fixture was not scraped")
			}
			official.fetchError, catalogue.fetchError = nil, nil
			official.complete, catalogue.complete = true, true
			// Ordinary scans intentionally preserve the old saved metadata.
			if _, err := f.library.StartScan(ctx); err != nil {
				t.Fatal(err)
			}
			f.runQueue(t)
			if official.queries != 1 || catalogue.queries != 1 {
				t.Fatal("ordinary scan repeated metadata requests")
			}
			job, err := f.library.StartRebuild(ctx)
			if err != nil || !job.Rebuild {
				t.Fatalf("start rebuild: %+v %v", job, err)
			}
			f.runQueue(t)
			after := f.store.Client.Movie.Query().WithActors().WithTags().OnlyX(ctx)
			if after.ID != before.ID || after.ScrapeStatus != movie.ScrapeStatusDone ||
				after.Metadata.FieldSources["title"] != "javdb" || after.Edges.Tags[0].Provider != "javdb" || after.Edges.Actors[0].Provider != "javdb" ||
				len(after.Metadata.Previews) != 2 || official.queries != 2 || catalogue.queries != 2 {
				t.Fatalf("rebuild reused old metadata: id=%d status=%s fields=%v previews=%d calls=%d/%d", after.ID, after.ScrapeStatus, after.Metadata.FieldSources, len(after.Metadata.Previews), official.queries, catalogue.queries)
			}
			local, err := f.library.Movie(ctx, after.ID)
			if err != nil || len(local.PreviewImages) != 2 || local.Tags[0].Provider != "javdb" {
				t.Fatalf("local detail not refreshed: %+v %v", local, err)
			}
			path := filepath.Join(scrape.EmbyMovieDir(f.embyDir, after.Code), after.Code+".nfo")
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := nfo.Decode(body)
			previews := 0
			for _, image := range doc.Images {
				if image.Role == "preview" {
					previews++
				}
			}
			if err != nil || previews != 2 || doc.Tags[0].Provider != "javdb" {
				t.Fatalf("export not refreshed: %+v %v", doc, err)
			}
			infos, err := f.library.ListTasks(ctx)
			if err != nil || infos[0].ID != job.ID || !infos[0].Rebuild || infos[0].Status != "done" || infos[0].Scan.MetadataCompleted != 1 {
				t.Fatalf("wrong rebuild progress: %+v %v", infos, err)
			}
		})
	}
}
