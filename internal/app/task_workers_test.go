package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ppxb/miyabi/internal/config"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestSubscriptionBatchesDoNotBlockLibraryWorkers(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	registry := tasks.NewRegistry()
	service := tasks.NewService(store.Client, registry)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	batchStarted := make(chan tasks.Job, 4)
	libraryStarted := make(chan tasks.Job, 6)
	batchGate := make(chan struct{})
	scanGate := make(chan struct{})
	var activeLibrary, activeBatches atomic.Int32
	var overlapped atomic.Bool
	registry.Register(tasks.NewHandler(tasks.KindSubscriptionBatch, func(ctx context.Context, job tasks.Job) error {
		if activeBatches.Add(1) != 1 {
			overlapped.Store(true)
		}
		defer activeBatches.Add(-1)
		batchStarted <- job
		select {
		case <-batchGate:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, func(context.Context, *ent.Tx, tasks.Job, error) (tasks.Change, error) {
		return tasks.ChangeOffline | tasks.ChangeMonitor, nil
	}))
	for _, kind := range []tasks.Kind{tasks.KindScan, tasks.KindScrape, tasks.KindCover} {
		registry.Register(tasks.NewHandler(kind, func(ctx context.Context, job tasks.Job) error {
			if activeLibrary.Add(1) != 1 {
				overlapped.Store(true)
			}
			defer activeLibrary.Add(-1)
			libraryStarted <- job
			if job.Type == tasks.KindScan {
				select {
				case <-scanGate:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		}, func(context.Context, *ent.Tx, tasks.Job, error) (tasks.Change, error) {
			return tasks.ChangeLibrary, nil
		}))
	}
	checkpoint := json.RawMessage(`{"ids":[10,20],"batch":{"total":2,"processed":1,"submitted":1}}`)
	// Both pools must recover their own interrupted tasks without resetting the other pool.
	batch := store.Client.Task.Create().SetType(string(tasks.KindSubscriptionBatch)).SetStatus(task.StatusRunning).SetPayload(checkpoint).SaveX(ctx)
	secondBatch := store.Client.Task.Create().SetType(string(tasks.KindSubscriptionBatch)).SaveX(ctx)
	scan := store.Client.Task.Create().SetType(string(tasks.KindScan)).SetStatus(task.StatusRunning).SaveX(ctx)
	scrape := store.Client.Task.Create().SetType(string(tasks.KindScrape)).SaveX(ctx)
	cover := store.Client.Task.Create().SetType(string(tasks.KindCover)).SaveX(ctx)
	pools := newTaskPools(service, config.DefaultRuntime().TaskPoolWorkers, logger)
	runCtx, cancel := context.WithCancel(ctx)
	stopped := make(chan error, 2)
	var workers sync.WaitGroup
	var stop context.CancelFunc
	startPool := func(pool *tasks.Pool, ctx context.Context) {
		workers.Add(1)
		go func() { defer workers.Done(); stopped <- pool.Run(ctx) }()
	}
	t.Cleanup(func() {
		cancel()
		if stop != nil {
			stop()
		}
		done := make(chan struct{})
		go func() { workers.Wait(); close(done) }()
		awaitPan(t, done)
	})
	startPool(pools[1], runCtx)
	first := awaitPan(t, batchStarted)
	if first.ID != batch.ID || string(first.Payload) != string(checkpoint) {
		t.Fatalf("batch recovery lost its checkpoint: %+v", first)
	}
	startPool(pools[0], runCtx)
	if started := awaitPan(t, libraryStarted); started.ID != scan.ID {
		t.Fatalf("wrong library task: %+v", started)
	}
	if got := store.Client.Task.GetX(ctx, batch.ID); got.Status != task.StatusRunning {
		t.Fatalf("library recovery reset the running batch: %+v", got)
	}
	if got := store.Client.Task.GetX(ctx, secondBatch.ID); got.Status != task.StatusQueued {
		t.Fatalf("batches ran concurrently: %+v", got)
	}
	close(scanGate)
	for _, id := range []int{scrape.ID, cover.ID} {
		if started := awaitPan(t, libraryStarted); started.ID != id {
			t.Fatalf("library order changed: %+v, want %d", started, id)
		}
	}
	awaitPanCondition(t, func() bool { return store.Client.Task.GetX(ctx, cover.ID).Status == task.StatusDone })
	if activeBatches.Load() != 1 || overlapped.Load() {
		t.Fatal("library work blocked on the batch or a serial pool overlapped handlers")
	}
	cancel()
	for range 2 {
		if err := awaitPan(t, stopped); err != nil {
			t.Fatal(err)
		}
	}
	if got := store.Client.Task.GetX(ctx, batch.ID); got.Status != task.StatusRunning || string(got.Payload) != string(checkpoint) {
		t.Fatalf("shutdown failed or lost the interrupted batch: %+v", got)
	}
	// Restart and finish both batches, preserving their order and completion hooks.
	close(batchGate)
	resumedCtx, stop := context.WithCancel(ctx)
	defer stop()
	for _, pool := range newTaskPools(service, config.DefaultRuntime().TaskPoolWorkers, logger) {
		startPool(pool, resumedCtx)
	}
	for _, id := range []int{batch.ID, secondBatch.ID} {
		if started := awaitPan(t, batchStarted); started.ID != id {
			t.Fatalf("batch order changed after restart: %+v", started)
		}
	}
	awaitPanCondition(t, func() bool {
		return store.Client.Task.GetX(ctx, secondBatch.ID).Status == task.StatusDone && service.Revisions().Monitor == 2
	})
	if rev := service.Revisions(); rev.Library != 3 || rev.Offline != 2 || rev.Monitor != 2 {
		t.Fatalf("completion hooks changed: %+v", rev)
	}
	// Each idle pool must wake for newly queued work using the same shared bus.
	newScan := store.Client.Task.Create().SetType(string(tasks.KindScan)).SaveX(ctx)
	service.Notify()
	if started := awaitPan(t, libraryStarted); started.ID != newScan.ID {
		t.Fatalf("library pool missed enqueue: %+v", started)
	}
	awaitPanCondition(t, func() bool { return store.Client.Task.GetX(ctx, newScan.ID).Status == task.StatusDone })
	newBatch := store.Client.Task.Create().SetType(string(tasks.KindSubscriptionBatch)).SaveX(ctx)
	service.Notify()
	if started := awaitPan(t, batchStarted); started.ID != newBatch.ID {
		t.Fatalf("batch pool missed enqueue: %+v", started)
	}
	awaitPanCondition(t, func() bool { return store.Client.Task.GetX(ctx, newBatch.ID).Status == task.StatusDone })
	stop()
	for range 2 {
		if err := awaitPan(t, stopped); err != nil {
			t.Fatal(err)
		}
	}
	if overlapped.Load() {
		t.Fatal("a serial pool ran multiple handlers concurrently")
	}
}
