package scan

import (
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/export"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestScanNotificationsFollowTransactionCommit(t *testing.T) {
	for _, operation := range []string{"page", "reconcile", "reconcile with scrape"} {
		for _, commit := range []bool{false, true} {
			outcome := "rollback"
			if commit {
				outcome = "commit"
			}
			t.Run(operation+"/"+outcome, func(t *testing.T) {
				ctx := t.Context()
				store, err := database.Open(ctx, t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				defer store.Close()
				svc := tasks.NewService(store.Client, tasks.NewRegistry())
				updates, unsubscribe := svc.Subscribe()
				defer unsubscribe()
				work, stopWork := svc.SubscribePool()
				defer stopWork()
				job := store.Client.Task.Create().SetType(tasks.KindScan.String()).SaveX(ctx)
				payload := domain.ScanPayload{ScanID: "current", Source: domain.LibrarySource{
					AccountID: "account", Directory: domain.LibraryDirectory{ID: "root"},
				}}
				if operation == "reconcile" {
					store.Client.File.Create().SetFileID("stale").SetName("old.mp4").SetSize(1).
						SetAccountID("account").SetRootID("root").SetScanID("previous").ExecX(ctx)
				}
				if operation == "reconcile with scrape" {
					film := store.Client.Movie.Create().SetCode("TEST-001").SaveX(ctx)
					store.Client.File.Create().SetFileID("indexed").SetName("TEST-001.mp4").SetSize(1).
						SetAccountID("account").SetRootID("root").SetScanID(payload.ScanID).SetMovieID(film.ID).ExecX(ctx)
				}
				tx, err := store.Client.Tx(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				run := scanRun{scanner: &Scanner{tasksSvc: svc}, taskID: job.ID, payload: &payload}
				if operation == "page" {
					err = run.processPageTx(ctx, tx, "/Movies", []Video{{File: pan.File{ID: "new", Name: "new.mp4", Size: 1}}}, nil)
				} else {
					err = run.reconcileTx(ctx, tx, export.Config{})
				}
				if err != nil {
					t.Fatal(err)
				}
				if got := svc.Revisions().Library; got != 0 {
					t.Fatalf("published library revision before commit: %d", got)
				}
				if len(work) != 0 {
					t.Fatal("worker woke before scan transaction committed")
				}
				select {
				case <-updates:
					t.Fatal("woke subscribers before commit")
				default:
				}
				if commit {
					err = tx.Commit()
				} else {
					err = tx.Rollback()
				}
				if err != nil {
					t.Fatal(err)
				}
				wantRevision := uint64(0)
				if commit && operation != "reconcile with scrape" {
					wantRevision = 1
				}
				if got := svc.Revisions().Library; got != wantRevision {
					t.Fatalf("library revision = %d, want %d", got, wantRevision)
				}
				select {
				case <-updates:
					if !commit {
						t.Fatal("rollback woke subscribers")
					}
				default:
					if commit {
						t.Fatal("commit did not wake subscribers")
					}
				}
				wantFiles := 0
				if operation == "page" && commit || operation == "reconcile" && !commit || operation == "reconcile with scrape" {
					wantFiles = 1
				}
				if got := store.Client.File.Query().CountX(ctx); got != wantFiles {
					t.Fatalf("committed file count = %d, want %d", got, wantFiles)
				}
				wantWork := 0
				if commit && operation == "reconcile with scrape" {
					wantWork = 1
				}
				if len(work) != wantWork {
					t.Fatalf("worker notifications = %d, want %d", len(work), wantWork)
				}
			})
		}
	}
}
