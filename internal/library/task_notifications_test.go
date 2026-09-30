package library

import (
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/task"
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
