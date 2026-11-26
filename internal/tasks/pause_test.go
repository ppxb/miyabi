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
	// Pausing the library must not hold back a task of any other kind.
	const otherKind Kind = "other"
	other := store.Client.Task.Create().SetType(string(otherKind)).SaveX(ctx)
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
	if job, err := svc.Queue().Claim(ctx, []Kind{otherKind}); err != nil || job == nil || job.ID != other.ID {
		t.Fatalf("pause affected another task kind: %+v %v", job, err)
	}
	if err := svc.SetLibraryPaused(ctx, false); err != nil {
		t.Fatal(err)
	}
	if job, err := svc.Queue().Claim(ctx, []Kind{KindScan}); err != nil || job == nil || job.ID != scan.ID {
		t.Fatalf("resume lost original task: %+v %v", job, err)
	}
}

func TestFinishWakesOnlyItsOwnPool(t *testing.T) {
	for _, test := range []struct {
		kind       Kind
		wantScan   int
		wantScrape int
	}{
		{KindScan, 1, 0},
		{KindScrape, 0, 1},
	} {
		t.Run(string(test.kind), func(t *testing.T) {
			ctx := t.Context()
			store, err := database.Open(ctx, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			svc := NewService(store.Client, NewRegistry())
			scan, stopScan := svc.SubscribePool(KindScan)
			defer stopScan()
			scrape, stopScrape := svc.SubscribePool(KindScrape)
			defer stopScrape()

			record := store.Client.Task.Create().SetType(string(test.kind)).SaveX(ctx)
			if err := svc.Queue().Finish(ctx, record.ID, nil); err != nil {
				t.Fatal(err)
			}
			if len(scan) != test.wantScan || len(scrape) != test.wantScrape {
				t.Fatalf("finish %s wake: scan=%d scrape=%d", test.kind, len(scan), len(scrape))
			}
		})
	}
}

func TestSetLibraryPausedUpdatesCachedState(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := NewService(store.Client, NewRegistry())
	// Prime the cache with the persisted (unpaused) value.
	if job, err := svc.Queue().Claim(ctx, []Kind{KindScan}); err != nil || job != nil {
		t.Fatalf("prime claim: %+v %v", job, err)
	}
	store.Client.Task.Create().SetType(string(KindScan)).SaveX(ctx)
	if err := svc.SetLibraryPaused(ctx, true); err != nil {
		t.Fatal(err)
	}
	// The same service must observe the toggle without reloading from the DB.
	if job, err := svc.Queue().Claim(ctx, []Kind{KindScan}); err != nil || job != nil {
		t.Fatalf("cached pause ignored: %+v %v", job, err)
	}
}

func TestPauseToggleWakesBothLibraryPools(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := NewService(store.Client, NewRegistry())
	scan, stopScan := svc.SubscribePool(KindScan)
	defer stopScan()
	scrape, stopScrape := svc.SubscribePool(KindScrape)
	defer stopScrape()

	if err := svc.SetLibraryPaused(ctx, false); err != nil {
		t.Fatal(err)
	}
	if len(scan) != 1 || len(scrape) != 1 {
		t.Fatalf("pause toggle wake: scan=%d scrape=%d", len(scan), len(scrape))
	}
}
