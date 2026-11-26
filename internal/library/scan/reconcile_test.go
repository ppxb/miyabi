package scan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestRescanReusesActiveScrapeWithoutChangingItsCheckpoint(t *testing.T) {
	for _, scenario := range []string{"queued", "running", "retrying", "other source", "failed", "published"} {
		t.Run(scenario, func(t *testing.T) {
			run := reconcileFixture(t, 0, 1)
			ctx := t.Context()
			film := run.scanner.db.Movie.Query().OnlyX(ctx)
			source := run.payload.Source
			if scenario == "other source" {
				source.AccountID = "another-account"
			}
			body, err := tasks.EncodePayload(scrape.Payload{MetadataPayload: scrape.MetadataPayload{Source: source, MovieID: film.ID, Code: film.Code, ScanTaskID: 900}, MetadataReady: true, Completed: scenario == "published"})
			if err != nil {
				t.Fatal(err)
			}
			builder := run.scanner.db.Task.Create().SetType("scrape").SetResourceKey(fmt.Sprintf("movie:%d", film.ID)).SetPayload(body).SetProgress(45)
			if scenario == "running" {
				builder.SetStatus(task.StatusRunning)
			}
			if scenario == "retrying" {
				builder.SetRetryCount(2).SetRetryAt(time.Now().Add(time.Hour))
			}
			if scenario == "failed" {
				builder.SetStatus(task.StatusFailed)
			}
			original := builder.SaveX(ctx)
			if err := run.reconcile(ctx); err != nil {
				t.Fatal(err)
			}
			reused := scenario != "other source" && scenario != "failed" && scenario != "published"
			want := 2
			if reused {
				want = 1
			}
			if got := run.scanner.db.Task.Query().Where(task.TypeEQ("scrape")).CountX(ctx); got != want {
				t.Fatalf("scrape jobs=%d want=%d", got, want)
			}
			payload, err := tasks.DecodePayload[domain.ScanPayload](run.scanner.db.Task.GetX(ctx, run.taskID).Payload)
			if err != nil || reused && !slices.Equal(payload.ReusedTasks, []int{original.ID}) || !reused && len(payload.ReusedTasks) != 0 {
				t.Fatalf("wrong shared work: %+v %v", payload, err)
			}
			got := run.scanner.db.Task.GetX(ctx, original.ID)
			if string(got.Payload) != string(body) || got.Status != original.Status || got.Progress != 45 || got.RetryCount != original.RetryCount {
				t.Fatalf("rescan reset active work: %+v", got)
			}
		})
	}
}

func TestRebuildRefreshesCompletedMoviesAndNeverReusesOrdinaryScrapes(t *testing.T) {
	run := reconcileFixture(t, 2, 0)
	ctx := t.Context()
	run.payload.Rebuild = true
	film := run.scanner.db.Movie.Query().Order(movie.ByID()).FirstX(ctx)
	body, _ := tasks.EncodePayload(scrape.Payload{MetadataPayload: scrape.MetadataPayload{
		Source: run.payload.Source, MovieID: film.ID, Code: film.Code, ScanTaskID: 900,
	}, MetadataReady: true})
	old := run.scanner.db.Task.Create().SetType("scrape").SetResourceKey(fmt.Sprintf("movie:%d", film.ID)).SetPayload(body).SaveX(ctx)
	for range 2 {
		if err := run.reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	jobs := run.scanner.db.Task.Query().Where(task.TypeEQ("scrape"), task.IDNEQ(old.ID)).AllX(ctx)
	if len(jobs) != 2 {
		t.Fatalf("rebuild jobs = %d, want 2", len(jobs))
	}
	for _, job := range jobs {
		input, err := tasks.DecodePayload[scrape.Payload](job.Payload)
		if err != nil || !input.Rebuild || input.MetadataReady || input.Artwork != nil {
			t.Fatalf("rebuild retained an old checkpoint: %+v %v", input, err)
		}
	}
	if got := run.scanner.db.Task.GetX(ctx, old.ID); string(got.Payload) != string(body) {
		t.Fatal("rebuild changed another task's checkpoint")
	}
}

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
	artwork, err := images.FromCover(testJPEG(t), "single")
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
				builder.SetMetadata(&nfo.Movie{Code: code}).SetScrapeStatus(movie.ScrapeStatusDone).SetPoster(artwork.Poster).SetCover(artwork.Thumbnail).
					SetFanarts([]string{artwork.Fanart}).SetMetadataSnapshot(&domain.MetadataSnapshot{
					AccountID: payload.Source.AccountID, DirectoryID: payload.Source.Directory.ID,
					Videos: scrape.VideoFingerprint([]pan.File{video}), PosterVersion: mediaimage.PosterVersion,
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
		if err != nil || input.ScanTaskID != run.taskID || seen[input.MovieID] || input.Source != run.payload.Source || job.ResourceKey != fmt.Sprintf("movie:%d", input.MovieID) {
			t.Fatalf("invalid or duplicate child: %+v %v", input, err)
		}
		seen[input.MovieID] = true
	}
	for _, code := range []string{"TEST-000", "TEST-100", "TEST-200"} {
		if _, err := os.Stat(filepath.Join(export.EmbyMovieDir(run.scanner.exportMgr.Config().EmbyDir, code), code+".strm")); err != nil {
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
	exported := filepath.Join(export.EmbyMovieDir(run.scanner.exportMgr.Config().EmbyDir, "TEST-000"), "TEST-000.strm")
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

func TestReconcileNotifiesOnlyRewrittenExports(t *testing.T) {
	run := reconcileFixture(t, 2, 0)
	ctx := t.Context()
	embyDir := run.scanner.exportMgr.Config().EmbyDir
	var notified []string
	run.scanner.notifier = localNotifier(func(_ context.Context, _ *ent.Tx, path string) error {
		notified = append(notified, path)
		return nil
	})
	if err := run.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if len(notified) != 2 {
		t.Fatalf("first export notifications=%v, want both movies", notified)
	}
	// A complete export stays quiet; a damaged one is repaired and announced.
	nfoPath := filepath.Join(export.EmbyMovieDir(embyDir, "TEST-001"), "TEST-001.nfo")
	if err := os.WriteFile(nfoPath, []byte("damaged"), 0o644); err != nil {
		t.Fatal(err)
	}
	notified = nil
	if err := run.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if want := export.EmbyMovieDir(embyDir, "TEST-001"); len(notified) != 1 || notified[0] != want {
		t.Fatalf("notifications=%v, want only %s", notified, want)
	}
	completed, err := tasks.DecodePayload[domain.ScanPayload](run.scanner.db.Task.GetX(ctx, run.taskID).Payload)
	if err != nil || completed.Scan.Stage != "done" || len(completed.ExportedCodes) != 0 {
		t.Fatalf("completed scan kept exported codes: %+v %v", completed, err)
	}
}

func TestReconcileSavesExportsRewrittenBeforeFailure(t *testing.T) {
	run := reconcileFixture(t, 2, 0)
	ctx := t.Context()
	// TEST-001 writes its artwork, then fails on an NFO path taken by a directory.
	blocked := filepath.Join(export.EmbyMovieDir(run.scanner.exportMgr.Config().EmbyDir, "TEST-001"), "TEST-001.nfo")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run.reconcile(ctx); err == nil {
		t.Fatal("blocked export did not fail")
	}
	resumed, err := tasks.DecodePayload[domain.ScanPayload](run.scanner.db.Task.GetX(ctx, run.taskID).Payload)
	if err != nil || resumed.Scan.Stage != "reconciling" || !slices.Equal(resumed.ExportedCodes, []string{"TEST-000", "TEST-001"}) {
		t.Fatalf("failure lost rewritten exports: %+v %v", resumed, err)
	}
}

func TestIncrementalReconcileOnlyRestoresMissingExports(t *testing.T) {
	run := reconcileFixture(t, 1, 0)
	ctx := t.Context()
	nfoPath := filepath.Join(export.EmbyMovieDir(run.scanner.exportMgr.Config().EmbyDir, "TEST-000"), "TEST-000.nfo")
	reconcile := func(incremental bool) []byte {
		t.Helper()
		run.payload.Incremental = incremental
		if err := run.reconcile(ctx); err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(nfoPath)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	// A missing export directory, e.g. after switching the Emby directory, is restored.
	complete := reconcile(true)
	if err := os.WriteFile(nfoPath, []byte("damaged"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := reconcile(true); string(got) != "damaged" {
		t.Fatal("incremental sync compared and rewrote an existing export")
	}
	if got := reconcile(false); !bytes.Equal(got, complete) {
		t.Fatal("full scan did not repair the damaged export")
	}
	if run.scanner.db.Task.Query().Where(task.TypeEQ("scrape")).CountX(ctx) != 0 {
		t.Fatal("reconcile queued a completed movie")
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
			film.Update().SetMetadataSnapshot(&domain.MetadataSnapshot{AccountID: "account", DirectoryID: "root", Videos: scrape.VideoFingerprint(retained), PosterVersion: mediaimage.PosterVersion}).ExecX(ctx)
			if err := run.reconcile(ctx); err != nil {
				t.Fatal(err)
			}
			if got := run.scanner.db.Task.Query().Where(task.TypeEQ("scrape")).CountX(ctx); got != 0 {
				t.Fatal("stale or foreign files invalidated a reusable snapshot")
			}
			dir := export.EmbyMovieDir(run.scanner.exportMgr.Config().EmbyDir, film.Code)
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

type cleanupNotifier struct {
	export.MediaNotifier
	update func(context.Context, string) error
}

func (n cleanupNotifier) NotifyUpdated(ctx context.Context, path string) error {
	return n.update(ctx, path)
}

func TestReconcileRetriesReassignedExportCleanup(t *testing.T) {
	run := reconcileFixture(t, 0, 0)
	ctx := t.Context()
	run.payload.ObsoleteCodes = []string{"ONSD-340A", "ONSD-340B"}
	run.payload.Scan.RemovedMovies = 2
	if err := SaveScanProgress(ctx, run.scanner.db.Task, run.taskID, *run.payload); err != nil {
		t.Fatal(err)
	}
	for _, code := range run.payload.ObsoleteCodes {
		if _, err := export.WriteMedia(export.Config{EmbyDir: run.scanner.exportMgr.Config().EmbyDir, PublicURL: "http://localhost", STRMToken: ""}, code,
			nfo.Movie{Code: code}, []pan.File{{ID: code, Name: code + ".mp4"}}, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	failure := errors.New("notification unavailable")
	fail, notifications := true, 0
	run.scanner.notifier = cleanupNotifier{update: func(context.Context, string) error {
		notifications++
		if fail {
			return failure
		}
		return nil
	}}
	if err := run.reconcile(ctx); !errors.Is(err, failure) {
		t.Fatalf("cleanup error = %v", err)
	}
	resumed, err := tasks.DecodePayload[domain.ScanPayload](run.scanner.db.Task.GetX(ctx, run.taskID).Payload)
	if err != nil || resumed.Scan.Stage != "reconciling" || len(resumed.ObsoleteCodes) != 2 {
		t.Fatalf("cleanup lost its checkpoint: %+v, %v", resumed, err)
	}
	run.payload = &resumed
	fail = false
	if err := run.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	completed, err := tasks.DecodePayload[domain.ScanPayload](run.scanner.db.Task.GetX(ctx, run.taskID).Payload)
	if err != nil || completed.Scan.Stage != "done" || len(completed.ObsoleteCodes) != 0 || completed.Scan.RemovedMovies != 2 {
		t.Fatalf("cleanup did not finish: %+v, %v", completed, err)
	}
	if notifications != 3 {
		t.Fatalf("cleanup notifications = %d, want 3 including retry", notifications)
	}
	for _, code := range []string{"ONSD-340A", "ONSD-340B"} {
		if _, err := os.Stat(export.EmbyMovieDir(run.scanner.exportMgr.Config().EmbyDir, code)); !os.IsNotExist(err) {
			t.Fatalf("obsolete export %s remains: %v", code, err)
		}
	}
}

func TestReassignedExportCleanupPreservesRecreatedMovie(t *testing.T) {
	run := reconcileFixture(t, 0, 0)
	ctx, code := t.Context(), "ONSD-340A"
	run.payload.ObsoleteCodes = []string{code}
	run.scanner.db.Movie.Create().SetCode(code).ExecX(ctx)
	cfg := run.scanner.exportMgr.Config()
	if _, err := export.WriteMedia(export.Config{EmbyDir: cfg.EmbyDir, PublicURL: cfg.PublicURL, STRMToken: ""}, code,
		nfo.Movie{Code: code}, []pan.File{{ID: "retained", Name: code + ".mp4"}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := run.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	strm := filepath.Join(export.EmbyMovieDir(cfg.EmbyDir, code), code+".strm")
	if body, err := os.ReadFile(strm); err != nil || !strings.Contains(string(body), "retained") {
		t.Fatalf("cleanup deleted a recreated movie's export: %s, %v", body, err)
	}
}
