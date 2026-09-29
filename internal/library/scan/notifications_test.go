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
	for _, operation := range []string{"page", "reconcile"} {
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
				job := store.Client.Task.Create().SetType(tasks.KindScan.String()).SaveX(ctx)
				payload := domain.ScanPayload{ScanID: "current", Source: domain.LibrarySource{
					AccountID: "account", Directory: domain.LibraryDirectory{ID: "root"},
				}}
				if operation == "reconcile" {
					store.Client.File.Create().SetFileID("stale").SetName("old.mp4").SetSize(1).
						SetAccountID("account").SetRootID("root").SetScanID("previous").ExecX(ctx)
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
				if commit {
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
				if operation == "page" && commit || operation == "reconcile" && !commit {
					wantFiles = 1
				}
				if got := store.Client.File.Query().CountX(ctx); got != wantFiles {
					t.Fatalf("committed file count = %d, want %d", got, wantFiles)
				}
			})
		}
	}
}
