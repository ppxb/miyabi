package library

import (
	"context"
	"errors"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestScanEnqueueAndRetryWakeWorkers(t *testing.T) {
	lib, queued, payload := libraryFixture(t)
	work, stop := lib.tasks.SubscribePool()
	defer stop()
	if _, err := lib.EnqueueScan(t.Context(), payload.Source); err != nil {
		t.Fatal(err)
	}
	if len(work) != 1 {
		t.Fatal("queued scan did not wake workers")
	}
	<-work
	lib.database.Task.UpdateOneID(queued.ID).SetStatus(task.StatusRunning).ExecX(t.Context())
	if _, err := lib.EnqueueScan(t.Context(), payload.Source); err != nil {
		t.Fatal(err)
	}
	if len(work) != 0 {
		t.Fatal("reusing a running scan woke idle workers")
	}
	if _, err := lib.EnqueueFreshScan(t.Context(), payload.Source); err != nil {
		t.Fatal(err)
	}
	if len(work) != 1 {
		t.Fatal("fresh scan did not wake workers")
	}
	<-work
	lib.database.Task.UpdateOneID(queued.ID).SetStatus(task.StatusFailed).ExecX(t.Context())
	if _, err := lib.RetryTask(t.Context(), queued.ID); err != nil {
		t.Fatal(err)
	}
	if len(work) != 1 {
		t.Fatal("retry did not wake workers")
	}
}

func TestScrapeWakesWorkersAfterArtworkTaskCommits(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		name := "commit"
		if rollback {
			name = "rollback"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newCompletedScanFixture(t)
			lib := fixture.lib
			payload, err := tasks.EncodePayload(scrape.MetadataPayload{Source: fixture.payload.Source, ScanTaskID: fixture.queued.ID, MovieID: fixture.movie.ID, Code: fixture.movie.Code})
			if err != nil {
				t.Fatal(err)
			}
			job := lib.database.Task.Create().SetType(tasks.KindScrape.String()).SetPayload(payload).SaveX(t.Context())
			failure := errors.New("fixture artwork task creation failed")
			if rollback {
				lib.database.Task.Use(func(next ent.Mutator) ent.Mutator {
					return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
						if mutation.Op().Is(ent.OpCreate) {
							return nil, failure
						}
						return next.Mutate(ctx, mutation)
					})
				})
			}
			work, stop := lib.tasks.SubscribePool()
			defer stop()
			version := lib.tasks.Version()
			handler, _ := lib.tasks.Registry().Get(tasks.KindScrape)
			err = handler.Handle(t.Context(), tasks.Job{ID: job.ID, Type: tasks.KindScrape, Payload: payload})
			if rollback {
				if !errors.Is(err, failure) {
					t.Fatalf("scrape failure = %v", err)
				}
				if len(work) != 0 || lib.tasks.Version() != version {
					t.Fatal("failed scrape transaction notified subscribers")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if len(work) != 1 || lib.tasks.Version() != version+1 {
					t.Fatal("committed artwork task missed worker or UI notification")
				}
			}
		})
	}
}

func TestTargetedScanWakesWorkersOnlyAfterCommit(t *testing.T) {
	for _, commit := range []bool{false, true} {
		name := "rollback"
		if commit {
			name = "commit"
		}
		t.Run(name, func(t *testing.T) {
			store, err := database.Open(t.Context(), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			taskSvc := tasks.NewService(store.Client, tasks.NewRegistry())
			service := &Service{tasks: taskSvc}
			work, stop := taskSvc.SubscribePool()
			defer stop()
			tx, err := store.Client.Tx(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err := service.EnqueueTargetedScan(t.Context(), tx, domain.LibrarySource{}, "folder", 1, "TEST-001", "movie"); err != nil {
				t.Fatal(err)
			}
			if len(work) != 0 {
				t.Fatal("targeted scan woke workers before commit")
			}
			if commit {
				err = tx.Commit()
			} else {
				err = tx.Rollback()
			}
			if err != nil {
				t.Fatal(err)
			}
			if (len(work) == 1) != commit {
				t.Fatalf("worker wake after commit=%t: %d", commit, len(work))
			}
		})
	}
}
