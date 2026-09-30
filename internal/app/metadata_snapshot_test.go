package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	scrapePkg "github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestCoverSnapshotAndRecoveryCheckpointCommitTogether(t *testing.T) {
	for _, failCommit := range []bool{false, true} {
		name := "committed"
		if failCommit {
			name = "rollback then retry"
		}
		t.Run(name, func(t *testing.T) {
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
					t.Fatalf("claim metadata: %+v, %v", job, err)
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
				t.Fatalf("claim cover: %+v, %v", job, err)
			}
			rollback := errors.New("fixture checkpoint failure")
			fixture.store.Client.Task.Use(func(next ent.Mutator) ent.Mutator {
				return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
					if body, ok := mutation.(*ent.TaskMutation).Payload(); ok && failCommit {
						var input scrapePkg.CoverPayload
						if err := json.Unmarshal(body, &input); err != nil {
							return nil, err
						}
						if input.Completed {
							return nil, rollback
						}
					}
					return next.Mutate(ctx, mutation)
				})
			})
			err = fixture.scrape.Cover(ctx, *job)
			if failCommit {
				if !errors.Is(err, rollback) {
					t.Fatalf("checkpoint failure = %v", err)
				}
				record := fixture.store.Client.Movie.Query().OnlyX(ctx)
				if record.MetadataSnapshot != nil || record.ScrapeStatus != movie.ScrapeStatusPending {
					t.Fatalf("movie export escaped rollback: %+v", record)
				}
				failCommit = false
				saved := fixture.store.Client.Task.GetX(ctx, job.ID)
				input, err := tasks.DecodePayload[scrapePkg.CoverPayload](saved.Payload)
				if err != nil || input.Completed {
					t.Fatalf("checkpoint escaped rollback: %+v, %v", input, err)
				}
				job.Payload = saved.Payload
				if err := fixture.scrape.Cover(ctx, *job); err != nil {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			record := fixture.store.Client.Movie.Query().OnlyX(ctx)
			if record.MetadataSnapshot == nil || record.MetadataSnapshot.PosterVersion != mediaimage.PosterVersion || record.ScrapeStatus != movie.ScrapeStatusDone {
				t.Fatalf("export was not committed: %+v", record)
			}
			saved := fixture.store.Client.Task.GetX(ctx, job.ID)
			var checkpoint struct {
				Completed bool            `json:"completed"`
				Snapshot  json.RawMessage `json:"snapshot"`
			}
			if err := json.Unmarshal(saved.Payload, &checkpoint); err != nil || !checkpoint.Completed || checkpoint.Snapshot != nil {
				t.Fatalf("invalid checkpoint: %s, %v", saved.Payload, err)
			}
			// Simulate a crash after the export commit and before Queue.Finish.
			if err := fixture.tasks.Queue().Recover(ctx, []tasks.Kind{tasks.KindCover}); err != nil {
				t.Fatal(err)
			}
			resumed, err := fixture.tasks.Queue().Claim(ctx, []tasks.Kind{tasks.KindCover})
			if err != nil || resumed == nil || resumed.ID != job.ID {
				t.Fatalf("recover cover: %+v, %v", resumed, err)
			}
			fixture.drive.mu.Lock()
			delete(fixture.drive.files, "101") // Re-exporting would now fail position verification.
			fixture.drive.mu.Unlock()
			if err := fixture.scrape.Cover(ctx, *resumed); err != nil {
				t.Fatalf("completed cover ran again: %v", err)
			}
			if err := fixture.tasks.Queue().Finish(ctx, resumed.ID, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}
