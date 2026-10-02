package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/localsource"
	"github.com/ppxb/miyabi/internal/tasks"
)

func localSourceFixture(t *testing.T) (*Service, *tasks.Service, *localsource.Manager, string) {
	t.Helper()
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	registry := tasks.NewRegistry()
	queue := tasks.NewService(store.Client, registry)
	sources := localsource.NewManager(store.Client)
	lib := New(store.Client, nil, queue, nil, Options{LocalSources: sources})
	registry.Register(tasks.NewHandler(tasks.KindScan, lib.Scan, lib.Finished))
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ABC-123.strm"), []byte("https://example.com/video"), 0o600); err != nil {
		t.Fatal(err)
	}
	return lib, queue, sources, root
}

func TestLocalSourceScanImportsConfiguredDirectory(t *testing.T) {
	ctx := t.Context()
	lib, queue, sources, root := localSourceFixture(t)
	source, err := sources.Add(ctx, "下载目录", root)
	if err != nil {
		t.Fatal(err)
	}

	info, err := lib.StartLocalScan(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if info.Source.AccountID != domain.LocalAccountID || info.Source.Directory.ID != source.ID ||
		info.Source.Directory.Path != source.Path || info.Source.Directory.Name != source.Name {
		t.Fatalf("queued scan source = %+v", info.Source)
	}
	// A second request reuses the queued scan of the same directory.
	if again, err := lib.StartLocalScan(ctx, source.ID); err != nil || again.ID != info.ID {
		t.Fatalf("duplicate scan = %+v, %v", again, err)
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
	if count := lib.database.File.Query().Where(file.AccountIDEQ(domain.LocalAccountID)).CountX(ctx); count != 1 {
		t.Fatalf("indexed local files: %d", count)
	}
	if count := lib.database.Movie.Query().CountX(ctx); count != 1 {
		t.Fatalf("imported movies: %d", count)
	}
	if count := lib.database.Task.Query().CountX(ctx); count != 1 {
		t.Fatalf("queued scans: %d", count)
	}
}

func TestLocalSourceScanRefusesDisabledAndRemovedDirectories(t *testing.T) {
	ctx := t.Context()
	lib, queue, sources, root := localSourceFixture(t)
	source, err := sources.Add(ctx, "下载目录", root)
	if err != nil {
		t.Fatal(err)
	}

	disabled, enabled := false, true
	if _, err := sources.Update(ctx, source.ID, localsource.Update{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.StartLocalScan(ctx, source.ID); !domain.IsKind(err, domain.KindNotFound) {
		t.Fatalf("disabled scan err = %v", err)
	}
	if _, err := lib.StartLocalScan(ctx, "src-missing"); !domain.IsKind(err, domain.KindNotFound) {
		t.Fatalf("unknown scan err = %v", err)
	}
	if _, err := sources.Update(ctx, source.ID, localsource.Update{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}

	info, err := lib.StartLocalScan(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	job, err := queue.Queue().Claim(ctx, []tasks.Kind{tasks.KindScan})
	if err != nil || job == nil {
		t.Fatalf("claim: %v %v", job, err)
	}
	// A queued scan must not read a directory that is no longer configured.
	if err := sources.Remove(ctx, source.ID); err != nil {
		t.Fatal(err)
	}
	if err := lib.Scan(ctx, *job); !domain.IsKind(err, domain.KindConflict) {
		t.Fatalf("stale scan err = %v", err)
	}
	lib.database.Task.UpdateOneID(info.ID).SetStatus(task.StatusFailed).ExecX(ctx)
	if _, err := lib.RetryTask(ctx, info.ID); !domain.IsKind(err, domain.KindConflict) {
		t.Fatalf("stale retry err = %v", err)
	}
}
