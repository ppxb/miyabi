package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/pan"
)

func mountDirectoryPage(id string) pan.FilePage {
	return pan.FilePage{Path: []pan.Directory{{ID: "0", Name: "Root"}, {ID: id, Name: "Movies" + id}}}
}

func panMountFixture(t *testing.T) (*LibraryService, *panStub) {
	t.Helper()
	library, client := panConcurrencyFixture(t)
	client.list = func(_ context.Context, _, id string, _, _ int) (pan.FilePage, error) {
		return mountDirectoryPage(id), nil
	}
	if err := library.drive.ClearDirectory(t.Context()); err != nil {
		t.Fatal(err)
	}
	library.database.Task.Delete().ExecX(t.Context())
	return library, client
}

func TestDirectoryMountQueuesOnceAndDoesNotInterruptTheSameMount(t *testing.T) {
	library, _ := panMountFixture(t)
	ctx := t.Context()
	drive := library.drive
	directory, err := drive.SelectDirectory(ctx, "20")
	if err != nil || directory.ID != "20" {
		t.Fatalf("mount: %+v err=%v", directory, err)
	}
	record := library.database.Task.Query().OnlyX(ctx)
	payload, err := decodeTaskPayload[scanPayload](record.Payload)
	if err != nil || record.Type != "scan" || record.Status != task.StatusQueued ||
		payload.Source.AccountID != "100" || payload.Source.Directory != directory || payload.TargetID != "" {
		t.Fatalf("mount did not queue a full scan of its source: %+v %+v err=%v", record, payload, err)
	}
	version := drive.snapshot().authorizationVersion
	for _, status := range []task.Status{task.StatusQueued, task.StatusRunning, task.StatusDone} {
		library.database.Task.UpdateOne(record).SetStatus(status).ExecX(ctx)
		if _, err := drive.SelectDirectory(ctx, "20"); err != nil {
			t.Fatal(err)
		}
		if drive.snapshot().authorizationVersion != version || library.database.Task.Query().CountX(ctx) != 1 {
			t.Fatalf("repeated mount interrupted or duplicated a %s scan", status)
		}
	}
	if _, err := drive.SelectDirectory(ctx, "30"); err != nil {
		t.Fatal(err)
	}
	if drive.snapshot().authorizationVersion != version+1 || library.database.Task.Query().CountX(ctx) != 2 {
		t.Fatal("changing directory did not queue exactly one new scan")
	}
	// Startup restores the durable mount and queued work without adding scans.
	restarted, err := NewPanService(ctx, library.database, library.tasks, pan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if restarted.snapshot().directory.ID != "30" || library.database.Task.Query().CountX(ctx) != 2 {
		t.Fatal("restart lost the mount or created another scan")
	}
}

func TestRemountReusesQueuedScansButReplacesOldRunningScans(t *testing.T) {
	for _, previousStatus := range []task.Status{task.StatusQueued, task.StatusRunning} {
		t.Run(string(previousStatus), func(t *testing.T) {
			library, _ := panMountFixture(t)
			ctx := t.Context()
			if _, err := library.drive.SelectDirectory(ctx, "20"); err != nil {
				t.Fatal(err)
			}
			previous := library.database.Task.Query().OnlyX(ctx)
			library.database.Task.UpdateOne(previous).SetStatus(previousStatus).ExecX(ctx)
			if _, err := library.drive.SelectDirectory(ctx, "30"); err != nil {
				t.Fatal(err)
			}
			if _, err := library.drive.SelectDirectory(ctx, "20"); err != nil {
				t.Fatal(err)
			}
			want := 2
			if previousStatus == task.StatusRunning {
				want = 3
			}
			if got := library.database.Task.Query().CountX(ctx); got != want {
				t.Fatalf("remount scan count=%d, want %d", got, want)
			}
			if previousStatus == task.StatusRunning {
				latest := library.database.Task.Query().Order(ent.Desc(task.FieldID)).FirstX(ctx)
				payload, err := decodeTaskPayload[scanPayload](latest.Payload)
				if err != nil || latest.Status != task.StatusQueued || payload.Source.Directory.ID != "20" {
					t.Fatalf("remount relies on the invalidated running scan: %+v err=%v", latest, err)
				}
			}
		})
	}
}

func TestMountRollsBackDirectoryWhenScanCannotBeQueued(t *testing.T) {
	library, _ := panMountFixture(t)
	ctx := t.Context()
	before := library.drive.snapshot()
	revision := library.tasks.Revisions()
	failure := errors.New("cannot save scan")
	library.database.Task.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if mutation.Op().Is(ent.OpCreate) {
				return nil, failure
			}
			return next.Mutate(ctx, mutation)
		})
	})
	if _, err := library.drive.SelectDirectory(ctx, "20"); !errors.Is(err, failure) {
		t.Fatalf("queue failure=%v", err)
	}
	source, err := loadLibrarySource(ctx, library.database)
	if err != nil || source != nil || library.drive.snapshot().directory != before.directory ||
		library.drive.snapshot().authorizationVersion != before.authorizationVersion ||
		library.database.Task.Query().CountX(ctx) != 0 || library.tasks.Revisions() != revision {
		t.Fatalf("failed mount left partial state: source=%+v err=%v", source, err)
	}
}

func TestLateMountRequestsCannotReplaceANewerSource(t *testing.T) {
	for _, next := range []string{"20", "30", "disconnect"} {
		t.Run(next, func(t *testing.T) {
			library, client := panMountFixture(t)
			ctx := t.Context()
			started := make(chan struct{}, 1)
			hold, release := panTestGate(t)
			var calls atomic.Int32
			client.list = func(_ context.Context, _, id string, _, _ int) (pan.FilePage, error) {
				if calls.Add(1) == 1 {
					started <- struct{}{}
					<-hold
				}
				return mountDirectoryPage(id), nil
			}
			finished := make(chan error, 1)
			go func() { _, err := library.drive.SelectDirectory(ctx, "20"); finished <- err }()
			awaitPan(t, started)
			if next == "disconnect" {
				if _, err := library.drive.Disconnect(ctx); err != nil {
					t.Fatal(err)
				}
			} else if _, err := library.drive.SelectDirectory(ctx, next); err != nil {
				t.Fatal(err)
			}
			version := library.drive.snapshot().authorizationVersion
			release()
			err := awaitPan(t, finished)
			wantID, wantTasks := next, 1
			switch next {
			case "20":
				if err != nil {
					t.Fatalf("same mount retry failed: %v", err)
				}
			case "30":
				if !errors.Is(err, errPanSourceChanged) {
					t.Fatalf("stale directory request=%v", err)
				}
			case "disconnect":
				wantID, wantTasks = "", 0
				if !errors.Is(err, pan.ErrUnauthorized) {
					t.Fatalf("stale account request=%v", err)
				}
			}
			if library.drive.snapshot().directory.ID != wantID ||
				library.drive.snapshot().authorizationVersion != version ||
				library.database.Task.Query().CountX(ctx) != wantTasks {
				t.Fatal("late request changed the source or added another scan")
			}
		})
	}
}

func TestWorkerWaitsForMountPublicationBeforeClaimingItsScan(t *testing.T) {
	library, _ := panMountFixture(t)
	ctx := t.Context()
	committed := make(chan struct{}, 1)
	hold, release := panTestGate(t)
	library.database.Task.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if mutation.Op().Is(ent.OpCreate) {
				tx, err := mutation.(*ent.TaskMutation).Tx()
				if err != nil {
					return nil, err
				}
				tx.OnCommit(func(next ent.Committer) ent.Committer {
					return ent.CommitFunc(func(ctx context.Context, tx *ent.Tx) error {
						if err := next.Commit(ctx, tx); err != nil {
							return err
						}
						committed <- struct{}{}
						<-hold
						return nil
					})
				})
			}
			return next.Mutate(ctx, mutation)
		})
	})
	finished := make(chan error, 1)
	go func() { _, err := library.drive.SelectDirectory(ctx, "20"); finished <- err }()
	awaitPan(t, committed)
	if library.database.Task.Query().CountX(ctx) != 1 || library.drive.snapshot().directory.ID != "" {
		t.Fatal("fixture did not pause between durable commit and source publication")
	}
	waitContext, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	if job, err := library.tasks.Claim(waitContext, []string{"scan"}); !errors.Is(err, context.DeadlineExceeded) || job != nil {
		t.Fatalf("worker claimed a scan before its source was published: %+v err=%v", job, err)
	}
	release()
	if err := awaitPan(t, finished); err != nil {
		t.Fatal(err)
	}
	job, err := library.tasks.Claim(ctx, []string{"scan"})
	if err != nil || job == nil || library.drive.snapshot().directory.ID != "20" {
		t.Fatalf("published scan is not claimable: %+v err=%v", job, err)
	}
}

func TestAutomaticScanFailureKeepsMountAndAllowsManualRetry(t *testing.T) {
	library, _ := panMountFixture(t)
	ctx := t.Context()
	if _, err := library.drive.SelectDirectory(ctx, "20"); err != nil {
		t.Fatal(err)
	}
	first := library.database.Task.Query().OnlyX(ctx)
	if err := library.tasks.Finish(ctx, first.ID, errors.New("115 unavailable")); err != nil {
		t.Fatal(err)
	}
	source, err := loadLibrarySource(ctx, library.database)
	if err != nil || source == nil || source.Directory.ID != "20" || library.drive.snapshot().directory.ID != "20" {
		t.Fatalf("scan failure discarded the mount: %+v err=%v", source, err)
	}
	retry, err := library.StartScan(ctx)
	if err != nil || retry.ID == first.ID || retry.Status != task.StatusQueued || retry.Source.Directory.ID != "20" {
		t.Fatalf("manual retry failed: %+v err=%v", retry, err)
	}
}
