package tasks

import (
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent/task"
)

func TestLibraryPausePersistsAndPreservesCheckpoints(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := NewService(store.Client, NewRegistry())
	scan := store.Client.Task.Create().SetType(string(KindScan)).SetProgress(48).SetRetryCount(1).SaveX(ctx)
	if job, err := svc.Queue().Claim(ctx, []Kind{KindScan}); err != nil || job == nil {
		t.Fatal(err)
	}
	if err := svc.SetLibraryPaused(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := Checkpoint(ctx, store.Client); err != ErrPaused {
		t.Fatalf("checkpoint did not yield: %v", err)
	}
	if err := svc.Queue().Finish(ctx, scan.ID, ErrPaused); err != nil {
		t.Fatal(err)
	}
	before := store.Client.Task.GetX(ctx, scan.ID)
	if before.Status != task.StatusQueued || before.Progress != 48 || before.RetryCount != 1 || before.Error != nil {
		t.Fatalf("pause discarded state: %+v", before)
	}
	store.Client.Task.Create().SetType(string(KindScrape)).SetRetryAt(time.Now().Add(time.Second)).SaveX(ctx)
	batch := store.Client.Task.Create().SetType(string(KindSubscriptionBatch)).SaveX(ctx)
	svc = NewService(store.Client, NewRegistry())
	if err := svc.Queue().Recover(ctx, []Kind{KindScan, KindScrape}); err != nil {
		t.Fatal(err)
	}
	if job, err := svc.Queue().Claim(ctx, []Kind{KindScan, KindScrape}); err != nil || job != nil {
		t.Fatalf("paused library ran: %+v %v", job, err)
	}
	if next, err := svc.Queue().NextRetry(ctx, []Kind{KindScrape}); err != nil || !next.IsZero() {
		t.Fatalf("paused retry timer active: %v %v", next, err)
	}
	if job, err := svc.Queue().Claim(ctx, []Kind{KindSubscriptionBatch}); err != nil || job == nil || job.ID != batch.ID {
		t.Fatalf("pause affected submission: %+v %v", job, err)
	}
	if err := svc.SetLibraryPaused(ctx, false); err != nil {
		t.Fatal(err)
	}
	if job, err := svc.Queue().Claim(ctx, []Kind{KindScan}); err != nil || job == nil || job.ID != scan.ID {
		t.Fatalf("resume lost original task: %+v %v", job, err)
	}
}
