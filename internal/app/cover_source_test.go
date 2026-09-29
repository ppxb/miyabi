package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	scrapePkg "github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestCoverRecoversLegacyOriginAndRejectsChangedVideoPositions(t *testing.T) {
	for _, scenario := range []string{"legacy origin", "deleted video", "moved video"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPipelineFixture(t)
			ctx := t.Context()
			fixture.drive.addFile("101", "10", "ABP-123.mp4", 2<<30, []byte("video"))
			fixture.addCatalogueMovie(fixtureDetail())
			if _, err := fixture.library.StartScan(ctx); err != nil {
				t.Fatal(err)
			}
			for _, handle := range []func(context.Context, tasks.Job) error{fixture.library.Scan, fixture.scrape.Scrape} {
				job, err := fixture.tasks.Queue().Claim(ctx, []tasks.Kind{tasks.KindScan, tasks.KindScrape})
				if err != nil || job == nil {
					t.Fatalf("claim = %+v, %v", job, err)
				}
				if err := handle(ctx, *job); err != nil {
					t.Fatal(err)
				}
				if err := fixture.tasks.Queue().Finish(ctx, job.ID, nil); err != nil {
					t.Fatal(err)
				}
			}
			job, err := fixture.tasks.Queue().Claim(ctx, []tasks.Kind{tasks.KindCover})
			if err != nil || job == nil {
				t.Fatalf("claim cover = %+v, %v", job, err)
			}
			input, err := tasks.DecodePayload[scrapePkg.CoverPayload](job.Payload)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "legacy origin" {
				var stored map[string]json.RawMessage
				if err := json.Unmarshal(job.Payload, &stored); err != nil {
					t.Fatal(err)
				}
				delete(stored, "cover_url")
				stored["origin"] = json.RawMessage(`{"poster":{"id":"missing","pick_code":"old-poster"}}`)
				stored["document"] = json.RawMessage(`{"code":"ABP-123","title":"Old remote title"}`)
				job.Payload, err = json.Marshal(stored)
				if err != nil {
					t.Fatal(err)
				}
				fixture.store.Client.Task.UpdateOneID(job.ID).SetPayload(job.Payload).ExecX(ctx)
			} else {
				// Populate the directory cache before the remote file changes.
				sess, err := fixture.driveService.OpenSource(ctx, fixture.source)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := fixture.scrape.Directories(ctx, sess, input.MetadataPayload); err != nil {
					t.Fatal(err)
				}
				fixture.drive.addDirectory("11", "10", "moved")
				fixture.drive.mu.Lock()
				if scenario == "deleted video" {
					delete(fixture.drive.files, "101")
				} else {
					entry := fixture.drive.files["101"]
					entry.ParentID = "11"
					fixture.drive.files["101"] = entry
				}
				fixture.drive.mu.Unlock()
			}
			err = fixture.scrape.Cover(ctx, *job)
			if scenario == "legacy origin" {
				if err != nil {
					t.Fatal(err)
				}
				record := fixture.store.Client.Movie.GetX(ctx, input.MovieID)
				if record.Title != fixtureDetail().Title || record.MetadataSnapshot == nil || len(fixture.drive.reads) != 0 {
					t.Fatalf("legacy cover did not use catalogue: %+v, reads=%v", record, fixture.drive.reads)
				}
			} else {
				if err == nil {
					t.Fatal("export accepted a deleted or moved video")
				}
				if fixture.store.Client.Movie.GetX(ctx, input.MovieID).MetadataSnapshot != nil {
					t.Fatal("invalid video committed a snapshot")
				}
				if _, err := os.Stat(filepath.Join(fixture.embyDir, "ABP", "ABP-123", "ABP-123.strm")); !os.IsNotExist(err) {
					t.Fatalf("invalid video exported STRM: %v", err)
				}
			}
		})
	}
}
