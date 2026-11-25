package app

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/tasks"
)

// Re-exporting a completed movie reuses its saved metadata. A failure on the way
// must leave it completed and untouched, not pending during the job or failed after.
func TestFailedReexportKeepsCompletedMovie(t *testing.T) {
	f := newPipelineFixture(t)
	ctx := t.Context()
	f.drive.addFile("101", "10", "ABP-123.mp4", 2<<30, []byte("video"))
	f.addCatalogueMovie(fixtureDetail())
	if _, err := f.library.StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	f.runQueue(t)
	film := f.store.Client.Movie.Query().OnlyX(ctx)
	if film.ScrapeStatus != movie.ScrapeStatusDone {
		t.Fatalf("initial scrape status = %v", film.ScrapeStatus)
	}

	payload, err := tasks.EncodePayload(scrape.MetadataPayload{Source: f.source, MovieID: film.ID, Code: film.Code})
	if err != nil {
		t.Fatal(err)
	}
	job := f.store.Client.Task.Create().SetType(tasks.KindScrape.String()).
		SetResourceKey(fmt.Sprintf("movie:%d", film.ID)).SetPayload(payload).SaveX(ctx)
	sawPending := false
	f.store.Client.Task.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			m := mutation.(*ent.TaskMutation)
			id, _ := m.ID()
			if _, hasProgress := m.Progress(); id == job.ID && hasProgress {
				// Publication reports progress after metadata and artwork are ready.
				current := f.store.Client.Movie.GetX(ctx, film.ID)
				sawPending = current.ScrapeStatus != movie.ScrapeStatusDone
				return nil, errors.New("export interrupted")
			}
			return next.Mutate(ctx, mutation)
		})
	})
	f.runQueue(t)

	if failed := f.store.Client.Task.GetX(ctx, job.ID); failed.Status != task.StatusFailed {
		t.Fatalf("re-export task status = %v, want failed", failed.Status)
	}
	after := f.store.Client.Movie.GetX(ctx, film.ID)
	if sawPending || after.ScrapeStatus != movie.ScrapeStatusDone || !after.UpdatedAt.Equal(film.UpdatedAt) {
		t.Fatalf("completed movie changed: pending during job=%v status=%v updated %v -> %v",
			sawPending, after.ScrapeStatus, film.UpdatedAt, after.UpdatedAt)
	}
}
