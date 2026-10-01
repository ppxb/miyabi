package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestTaskPoolsIsolateScanningScrapingAndBatches(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	registry := tasks.NewRegistry()
	service := tasks.NewService(store.Client, registry)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	started := make(chan tasks.Job, 12)
	gate := make(chan struct{})
	var scans, scrapes, batches atomic.Int32
	var exceeded atomic.Bool
	for _, config := range []struct {
		kind  tasks.Kind
		count *atomic.Int32
		limit int32
	}{
		{tasks.KindScan, &scans, 1}, {tasks.KindScrape, &scrapes, 2}, {tasks.KindSubscriptionBatch, &batches, 1},
	} {
		registry.Register(tasks.NewHandler(config.kind, func(ctx context.Context, job tasks.Job) error {
			if config.count.Add(1) > config.limit {
				exceeded.Store(true)
			}
			defer config.count.Add(-1)
			started <- job
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-gate:
				return nil
			}
		}, func(context.Context, *ent.Tx, tasks.Job, error) (tasks.Change, error) {
			return tasks.ChangeLibrary, nil
		}))
	}
	body := json.RawMessage(`{"checkpoint":"saved"}`)
	// An older scrape backlog must not prevent a new scan or batch from starting.
	var jobs []*ent.Task
	for _, kind := range []tasks.Kind{tasks.KindScrape, tasks.KindScrape, tasks.KindScrape, tasks.KindScan, tasks.KindScan, tasks.KindSubscriptionBatch, tasks.KindSubscriptionBatch} {
		jobs = append(jobs, store.Client.Task.Create().SetType(string(kind)).SetPayload(body).SaveX(ctx))
	}
	jobs[0].Update().SetStatus(task.StatusRunning).ExecX(ctx)
	jobs[3].Update().SetStatus(task.StatusRunning).ExecX(ctx)
	jobs[5].Update().SetStatus(task.StatusRunning).ExecX(ctx)
	start := func() (context.CancelFunc, <-chan error) {
		runCtx, cancel := context.WithCancel(ctx)
		stopped := make(chan error, 3)
		for _, pool := range newTaskPools(service, logger) {
			go func() { stopped <- pool.Run(runCtx) }()
		}
		return cancel, stopped
	}
	stop := func(cancel context.CancelFunc, stopped <-chan error) {
		cancel()
		for range 3 {
			if err := awaitPan(t, stopped); err != nil {
				t.Error(err)
			}
		}
	}
	cancel, stopped := start()
	t.Cleanup(cancel)
	seen := make(map[int]bool)
	for range 4 {
		job := awaitPan(t, started)
		seen[job.ID] = true
		if string(job.Payload) != string(body) {
			t.Fatal("recovery discarded checkpoint")
		}
	}
	for _, i := range []int{0, 1, 3, 5} {
		if !seen[jobs[i].ID] {
			t.Fatalf("task %d starved", jobs[i].ID)
		}
	}
	for _, i := range []int{2, 4, 6} {
		if store.Client.Task.GetX(ctx, jobs[i].ID).Status != task.StatusQueued {
			t.Fatal("pool exceeded concurrency")
		}
	}
	stop(cancel, stopped)
	for id := range seen {
		if store.Client.Task.GetX(ctx, id).Status != task.StatusRunning {
			t.Fatal("shutdown marked an interrupted task failed")
		}
	}
	close(gate)
	cancel, stopped = start()
	t.Cleanup(cancel)
	for range len(jobs) {
		awaitPan(t, started)
	}
	awaitPanCondition(t, func() bool {
		// Notifications are published after the completion transaction commits.
		return store.Client.Task.Query().Where(task.StatusEQ(task.StatusDone)).CountX(ctx) == len(jobs) &&
			service.Revisions().Library >= uint64(len(jobs))
	})
	if service.Revisions().Library != uint64(len(jobs)) || exceeded.Load() {
		t.Fatal("duplicate completions or excess concurrency")
	}
	// Wake all idle pool kinds through the shared bus.
	for _, kind := range []tasks.Kind{tasks.KindScan, tasks.KindScrape, tasks.KindSubscriptionBatch} {
		job := store.Client.Task.Create().SetType(string(kind)).SaveX(ctx)
		service.WakePool()
		if got := awaitPan(t, started); got.ID != job.ID {
			t.Fatal("idle pool missed enqueue")
		}
		awaitPanCondition(t, func() bool { return store.Client.Task.GetX(ctx, job.ID).Status == task.StatusDone })
	}
	stop(cancel, stopped)
}
