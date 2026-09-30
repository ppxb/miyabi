package scan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/embynotification"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/export"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

func reconcileFixture(t *testing.T, completed, pending int) *scanRun {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	images, err := mediaimage.NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	artwork, err := images.FromCover(testJPEG(t))
	if err != nil {
		t.Fatal(err)
	}
	payload := &domain.ScanPayload{ScanID: "scan", Scan: domain.ScanProgress{Stage: "reconciling"},
		Source: domain.LibrarySource{AccountID: "account", Directory: domain.LibraryDirectory{ID: "root", Path: "/Movies"}}}
	encoded, err := tasks.EncodePayload(*payload)
	if err != nil {
		t.Fatal(err)
	}
	job := store.Client.Task.Create().SetType("scan").SetPayload(encoded).SaveX(t.Context())
	err = ent.WithTx(t.Context(), store.Client, func(tx *ent.Tx) error {
		for i := range completed + pending {
			code := fmt.Sprintf("TEST-%03d", i)
			video := pan.File{ID: fmt.Sprint(i), Name: code + ".mp4", ParentID: "folder", Size: 1024}
			builder := tx.Movie.Create().SetCode(code)
			if i < completed {
				builder.SetScrapeStatus(movie.ScrapeStatusDone).SetPoster(artwork.Poster).SetCover(artwork.Thumbnail).
					SetFanarts([]string{artwork.Fanart}).SetMetadataSnapshot(&domain.MetadataSnapshot{
					AccountID: payload.Source.AccountID, DirectoryID: payload.Source.Directory.ID,
					Videos: scrape.VideoFingerprint([]pan.File{video}),
				})
			}
			film := builder.SaveX(t.Context())
			tx.File.Create().SetFileID(video.ID).SetName(video.Name).SetParentID(video.ParentID).SetSize(video.Size).
				SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).
				SetScanID(payload.ScanID).SetPath("/Movies/" + video.Name).SetMovie(film).ExecX(t.Context())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := tasks.NewService(store.Client, tasks.NewRegistry())
	return &scanRun{scanner: &Scanner{db: store.Client, images: images, tasksSvc: svc,
		exportMgr: export.NewManager(export.Config{EmbyDir: t.TempDir(), PublicURL: "http://localhost", STRMToken: "token"})}, taskID: job.ID, payload: payload}
}

func TestReconcilePagesPreparationWithoutHoldingDatabaseWriter(t *testing.T) {
	run := reconcileFixture(t, 2*reconcileBatchSize+1, 2*reconcileBatchSize+5)
	ctx := t.Context()
	pages, inspected, writes := 0, 0, 0
	run.scanner.db.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			value, err := next.Query(ctx, query)
			if records, ok := value.([]*ent.Movie); ok && len(records) > 0 {
				if len(records) > reconcileBatchSize {
					t.Errorf("unbounded movie page: %d", len(records))
				}
				if records[0].ScrapeStatus == movie.ScrapeStatusDone {
					pages++
					inspected += len(records)
					writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
					defer cancel()
					if err := run.scanner.db.Movie.Create().SetCode(fmt.Sprintf("WRITER-%d", pages)).Exec(writeCtx); err != nil {
						t.Errorf("preparation held the database writer: %v", err)
					} else {
						writes++
					}
				}
			}
			return value, err
		})
	}))
	work, stop := run.scanner.tasksSvc.SubscribePool()
	defer stop()
	if err := run.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if pages != 3 || inspected != 201 || writes != 3 {
		t.Fatalf("preparation: pages=%d movies=%d concurrent writes=%d", pages, inspected, writes)
	}
	jobs := run.scanner.db.Task.Query().Where(task.TypeEQ("scrape")).AllX(ctx)
	if len(jobs) != 205 || len(work) != 1 {
		t.Fatalf("jobs=%d wakeups=%d", len(jobs), len(work))
	}
	seen := make(map[int]bool)
	for _, job := range jobs {
		input, err := tasks.DecodePayload[scrape.MetadataPayload](job.Payload)
		if err != nil || input.ScanTaskID != run.taskID || seen[input.MovieID] || input.Source != run.payload.Source {
			t.Fatalf("invalid or duplicate child: %+v %v", input, err)
		}
		seen[input.MovieID] = true
	}
	for _, code := range []string{"TEST-000", "TEST-100", "TEST-200"} {
		if _, err := os.Stat(filepath.Join(scrape.EmbyMovieDir(run.scanner.exportMgr.Config().EmbyDir, code), code+".strm")); err != nil {
			t.Fatalf("missing export %s: %v", code, err)
		}
	}
	if run.payload.Scan.Stage != "done" {
		t.Fatalf("stage=%s", run.payload.Scan.Stage)
	}
}

func TestReconcileRetryNotifiesExportsWrittenBeforeFailedCommit(t *testing.T) {
	run := reconcileFixture(t, 1, 1)
	ctx := t.Context()
	failure := errors.New("notification write failed")
	fail := true
	run.scanner.notifier = localNotifier(func(ctx context.Context, tx *ent.Tx, path string) error {
		if err := tx.EmbyNotification.Create().SetPath(path).OnConflictColumns(embynotification.FieldPath).UpdateNewValues().Exec(ctx); err != nil {
			return err
		}
		if count := run.scanner.db.EmbyNotification.Query().CountX(ctx); count != 0 {
			t.Error("notification became visible before commit")
		}
		if fail {
			return failure
		}
		return nil
	})
	work, stop := run.scanner.tasksSvc.SubscribePool()
	defer stop()
	if err := run.reconcile(ctx); !errors.Is(err, failure) {
		t.Fatalf("notification error=%v", err)
	}
	if run.scanner.db.Task.Query().Where(task.TypeEQ("scrape")).CountX(ctx) != 0 ||
		run.scanner.db.EmbyNotification.Query().CountX(ctx) != 0 || len(work) != 0 {
		t.Fatal("failed transaction retained children or notifications")
	}
	exported := filepath.Join(scrape.EmbyMovieDir(run.scanner.exportMgr.Config().EmbyDir, "TEST-000"), "TEST-000.strm")
	before, err := os.Stat(exported)
	if err != nil {
		t.Fatalf("export did not precede database commit: %v", err)
	}
	resumed, err := tasks.DecodePayload[domain.ScanPayload](run.scanner.db.Task.GetX(ctx, run.taskID).Payload)
	if err != nil || resumed.Scan.Stage != "reconciling" {
		t.Fatalf("failure lost resumable stage: %+v %v", resumed, err)
	}
	run.payload = &resumed
	fail = false
	if err := run.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(exported)
	if err != nil || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("retry rewrote an already complete export")
	}
	if run.scanner.db.EmbyNotification.Query().CountX(ctx) != 1 ||
		run.scanner.db.Task.Query().Where(task.TypeEQ("scrape")).CountX(ctx) != 1 || len(work) != 1 {
		t.Fatal("retry missed notification or duplicated metadata work")
	}
	completed := run.scanner.db.Task.GetX(ctx, run.taskID)
	if err := run.scanner.Run(ctx, tasks.Job{ID: completed.ID, Payload: completed.Payload}); err != nil {
		t.Fatalf("completed scan could not resume: %v", err)
	}
	if run.scanner.db.Task.Query().Where(task.TypeEQ("scrape")).CountX(ctx) != 1 {
		t.Fatal("completed scan replay duplicated metadata jobs")
	}
}

func TestReconcileRechecksPreparedMovieVersion(t *testing.T) {
	run := reconcileFixture(t, 1, 0)
	ctx := t.Context()
	cfg := run.scanner.exportMgr.Config()
	reusable, err := run.prepareReconcile(ctx, cfg)
	if err != nil || len(reusable) != 1 {
		t.Fatalf("prepare: %v %v", reusable, err)
	}
	film := run.scanner.db.Movie.Query().OnlyX(ctx)
	film.Update().SetScrapeStatus(movie.ScrapeStatusPending).SetTitle("changed").ExecX(ctx)
	if err := ent.WithTx(ctx, run.scanner.db, func(tx *ent.Tx) error {
		return run.reconcileTx(ctx, tx, cfg, reusable)
	}); err != nil {
		t.Fatal(err)
	}
	if got := run.scanner.db.Task.Query().Where(task.TypeEQ("scrape")).CountX(ctx); got != 1 {
		t.Fatal("stale preparation suppressed necessary metadata work")
	}
}

func TestReconcileBulkFailureRollsBackEarlierBatches(t *testing.T) {
	run := reconcileFixture(t, 0, reconcileBatchSize+1)
	ctx := t.Context()
	failure := errors.New("second task batch failed")
	created := 0
	run.scanner.db.Task.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if mutation.Op().Is(ent.OpCreate) {
				created++
				if created == reconcileBatchSize+1 {
					return nil, failure
				}
			}
			return next.Mutate(ctx, mutation)
		})
	})
	if err := run.reconcile(ctx); !errors.Is(err, failure) {
		t.Fatalf("bulk error=%v", err)
	}
	if created != reconcileBatchSize+1 || run.scanner.db.Task.Query().Where(task.TypeEQ("scrape")).CountX(ctx) != 0 {
		t.Fatal("task batches did not roll back atomically")
	}
	state, err := tasks.DecodePayload[domain.ScanPayload](run.scanner.db.Task.GetX(ctx, run.taskID).Payload)
	if err != nil || state.Scan.Stage != "reconciling" {
		t.Fatalf("failed bulk marked scan complete: %+v %v", state, err)
	}
}

func TestReconcileSnapshotUsesFilesRemainingAfterScopedCleanup(t *testing.T) {
	for _, targeted := range []bool{false, true} {
		t.Run(fmt.Sprint(targeted), func(t *testing.T) {
			run := reconcileFixture(t, 1, 0)
			ctx := t.Context()
			film := run.scanner.db.Movie.Query().OnlyX(ctx)
			if targeted {
				run.payload.TargetID, run.payload.TargetPath = "folder", "/Movies/Target"
			}
			retained := []pan.File{{ID: "0", Name: "TEST-000.mp4", ParentID: "folder", Size: 1024}}
			for _, entry := range []struct{ id, path, account string }{
				{"stale", "/Movies/Target/old.mp4", "account"},
				{"outside", "/Movies/Other/part.mp4", "account"},
				{"other-account", "/Movies/Other/part.mp4", "other"},
			} {
				video := pan.File{ID: entry.id, Name: entry.id + ".mp4", ParentID: "folder", Size: 1024}
				run.scanner.db.File.Create().SetFileID(video.ID).SetName(video.Name).SetParentID(video.ParentID).SetSize(video.Size).
					SetAccountID(entry.account).SetRootID("root").SetPath(entry.path).SetScanID("old").SetMovie(film).ExecX(ctx)
				if targeted && entry.id == "outside" {
					retained = append(retained, video)
				}
			}
			film.Update().SetMetadataSnapshot(&domain.MetadataSnapshot{AccountID: "account", DirectoryID: "root", Videos: scrape.VideoFingerprint(retained)}).ExecX(ctx)
			if err := run.reconcile(ctx); err != nil {
				t.Fatal(err)
			}
			if got := run.scanner.db.Task.Query().Where(task.TypeEQ("scrape")).CountX(ctx); got != 0 {
				t.Fatal("stale or foreign files invalidated a reusable snapshot")
			}
			dir := scrape.EmbyMovieDir(run.scanner.exportMgr.Config().EmbyDir, film.Code)
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			strms := 0
			for _, entry := range entries {
				if !strings.HasSuffix(entry.Name(), ".strm") {
					continue
				}
				strms++
				body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
				if err != nil || strings.Contains(string(body), "stale") || strings.Contains(string(body), "other-account") {
					t.Fatalf("export included removed or foreign video: %s, %v", body, err)
				}
			}
			if strms != len(retained) {
				t.Fatalf("STRMs=%d want=%d", strms, len(retained))
			}
		})
	}
}
