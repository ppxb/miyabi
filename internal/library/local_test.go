package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/export"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestLocalScanWorkflow(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	registry := tasks.NewRegistry()
	queue := tasks.NewService(store.Client, registry)
	lib := New(store.Client, nil, queue, nil)
	registry.Register(tasks.NewHandler(tasks.KindScan, lib.Scan, lib.Finished))
	root := t.TempDir()
	mgr := export.NewManager(export.Config{EmbyDir: root})
	lib.SetExportManager(mgr)
	if err := os.WriteFile(filepath.Join(root, "ABC-123.strm"), []byte("https://example.com/video"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := lib.ScheduleLocalScan(ctx); err != nil {
		t.Fatal(err)
	}
	info, err := lib.StartLocalScan(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if count := store.Client.Task.Query().CountX(ctx); count != 1 {
		t.Fatalf("duplicate tasks: %d", count)
	}
	if count := store.Client.Movie.Query().CountX(ctx); count != 0 {
		t.Fatalf("import ran before claim: %d", count)
	}
	if _, err := lib.StartLocalScan(ctx, t.TempDir()); err == nil {
		t.Fatal("accepted arbitrary directory")
	}
	job, err := queue.Queue().Claim(ctx, []tasks.Kind{tasks.KindScan})
	if err != nil || job == nil {
		t.Fatalf("claim: %v %v", job, err)
	}
	if err := lib.Scan(ctx, *job); err != nil {
		t.Fatal(err)
	}
	if err := queue.Queue().Finish(ctx, job.ID, nil); err != nil {
		t.Fatal(err)
	}
	listed, err := lib.ListTasks(ctx)
	if err != nil || len(listed) != 1 || listed[0].Status != "done" || listed[0].Scan.VideoFiles != 1 {
		t.Fatalf("tasks: %+v %v", listed, err)
	}
	if count := store.Client.Movie.Query().CountX(ctx); count != 1 {
		t.Fatalf("imported movies: %d", count)
	}

	// Local retries work without a 115 login and retain the original task ID.
	store.Client.Task.UpdateOneID(info.ID).SetStatus(task.StatusFailed).ExecX(ctx)
	retried, err := lib.RetryTask(ctx, info.ID)
	if err != nil || retried.ID != info.ID || retried.Status != "queued" {
		t.Fatalf("retry: %+v %v", retried, err)
	}
	job, err = queue.Queue().Claim(ctx, []tasks.Kind{tasks.KindScan})
	if err != nil || job == nil {
		t.Fatalf("claim retry: %v", err)
	}
	if err := lib.Scan(ctx, *job); err != nil {
		t.Fatal(err)
	}
	if count := store.Client.File.Query().CountX(ctx); count != 1 {
		t.Fatalf("rescan duplicated files: %d", count)
	}
	if err := queue.Queue().Finish(ctx, job.ID, nil); err != nil {
		t.Fatal(err)
	}

	// A queued scan cannot read an old directory after configuration changes.
	old, err := lib.StartLocalScan(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	oldJob, err := queue.Queue().Claim(ctx, []tasks.Kind{tasks.KindScan})
	if err != nil || oldJob == nil {
		t.Fatalf("claim old: %v", err)
	}
	mgr.Set(export.Config{EmbyDir: t.TempDir()})
	if err := lib.Scan(ctx, *oldJob); err == nil {
		t.Fatal("scanned stale directory")
	}
	store.Client.Task.UpdateOneID(old.ID).SetStatus(task.StatusFailed).ExecX(ctx)
	if _, err := lib.RetryTask(ctx, old.ID); err == nil {
		t.Fatal("retried stale directory")
	}
	current, err := lib.StartLocalScan(ctx, "")
	if err != nil || current.Source.Directory.ID == old.Source.Directory.ID {
		t.Fatalf("stale config: %+v %v", current, err)
	}

	// Waiting scans respect cancellation while another scan holds the lock.
	if err := lib.scanLock.Lock(ctx); err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithCancel(ctx)
	cancel()
	err = lib.Scan(waitCtx, *oldJob)
	lib.scanLock.Unlock()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting scan: %v", err)
	}

	mgr.Set(export.Config{EmbyDir: filepath.Join(t.TempDir(), "not-created")})
	if err := lib.ScheduleLocalScan(ctx); err != nil {
		t.Fatalf("fresh install: %v", err)
	}
}
