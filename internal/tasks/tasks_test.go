package tasks_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestPayloadUtilities(t *testing.T) {
	type sample struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	raw, err := tasks.EncodePayload(sample{Name: "test", Count: 42})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := tasks.DecodePayload[sample](raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Name != "test" || decoded.Count != 42 {
		t.Fatalf("unexpected decoded: %+v", decoded)
	}

	updated, err := tasks.SetPayloadField(raw, "extra", "field_value")
	if err != nil {
		t.Fatalf("set field: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(updated, &m); err != nil {
		t.Fatalf("unmarshal updated: %v", err)
	}
	if m["extra"] != "field_value" || m["name"] != "test" {
		t.Fatalf("unexpected updated payload: %+v", m)
	}

	if p := tasks.JSONExtract("payload", "source", "account_id"); p != "json_extract(payload, '$.source.account_id')" {
		t.Fatalf("json path: %s", p)
	}
	if p := tasks.JSONExtract("payload"); p != "json_extract(payload, '$')" {
		t.Fatalf("root json path: %s", p)
	}
}

func TestRegistryAndHandlers(t *testing.T) {
	registry := tasks.NewRegistry()

	var handled atomic.Bool
	var finishedHook atomic.Bool

	handler := tasks.NewHandler(
		tasks.KindScan,
		func(ctx context.Context, job tasks.Job) error {
			handled.Store(true)
			return nil
		},
		func(ctx context.Context, tx *ent.Tx, job tasks.Job, result error) (tasks.Change, error) {
			finishedHook.Store(true)
			return tasks.ChangeLibrary, nil
		},
	)

	registry.Register(handler)

	h, ok := registry.Get(tasks.KindScan)
	if !ok || h.Kind() != tasks.KindScan {
		t.Fatalf("get handler: ok=%v, kind=%v", ok, h.Kind())
	}

	kinds := registry.Kinds()
	if len(kinds) != 1 || kinds[0] != tasks.KindScan {
		t.Fatalf("unexpected kinds: %v", kinds)
	}

	if err := h.Handle(t.Context(), tasks.Job{ID: 1, Type: tasks.KindScan}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if !handled.Load() {
		t.Fatal("handler was not called")
	}

	hook, ok := h.(tasks.FinishedHook)
	if !ok {
		t.Fatal("handler does not implement FinishedHook")
	}
	if change, err := hook.Finished(t.Context(), nil, tasks.Job{ID: 1, Type: tasks.KindScan}, nil); err != nil || change != tasks.ChangeLibrary {
		t.Fatalf("finished: change=%v err=%v", change, err)
	}
	if !finishedHook.Load() {
		t.Fatal("finished hook was not called")
	}
}

func TestBusSubscriptionsAndRevisions(t *testing.T) {
	bus := tasks.NewBus()

	ch, unsubscribe := bus.Subscribe()
	defer unsubscribe()

	rev := bus.Revisions()
	if rev.Library != 0 || rev.Offline != 0 {
		t.Fatalf("initial revisions: %+v", rev)
	}

	bus.NotifyLibraryChanged()
	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("did not receive library update notification")
	}
	if bus.Revisions().Library != 1 {
		t.Fatalf("library revision not incremented: %+v", bus.Revisions())
	}

	bus.NotifyOfflineChanged()
	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("did not receive offline update notification")
	}
	if bus.Revisions().Offline != 1 {
		t.Fatalf("offline revision not incremented: %+v", bus.Revisions())
	}

	bus.NotifyMonitorChanged()
	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("did not receive monitor update notification")
	}
	if bus.Revisions().Monitor != 1 {
		t.Fatalf("monitor revision not incremented: %+v", bus.Revisions())
	}

	unsubscribe()
	bus.Notify()
	select {
	case <-ch:
		t.Fatal("received notification after unsubscribe")
	default:
	}
}

func TestQueueLifecycleAndHook(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	registry := tasks.NewRegistry()
	var hookResult error
	var hookCalled atomic.Bool

	registry.Register(tasks.NewHandler(
		tasks.KindScan,
		func(ctx context.Context, job tasks.Job) error { return nil },
		func(ctx context.Context, tx *ent.Tx, job tasks.Job, result error) (tasks.Change, error) {
			hookCalled.Store(true)
			hookResult = result
			return tasks.ChangeLibrary, nil
		},
	))

	svc := tasks.NewService(store.Client, registry)

	info, err := store.Client.Task.Create().SetType(string(tasks.KindScan)).SetPayload(json.RawMessage(`{}`)).Save(ctx)
	if err != nil {
		t.Fatalf("enqueue scan: %v", err)
	}
	if info.Status != task.StatusQueued {
		t.Fatalf("unexpected scan info: %+v", info)
	}

	job, err := svc.Queue().Claim(ctx, []tasks.Kind{tasks.KindScan})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if job == nil || job.ID != info.ID || job.Type != tasks.KindScan {
		t.Fatalf("unexpected claimed job: %+v", job)
	}

	if err := svc.Queue().Recover(ctx, []tasks.Kind{tasks.KindScan}); err != nil {
		t.Fatalf("recover: %v", err)
	}
	rec, err := store.Client.Task.Get(ctx, info.ID)
	if err != nil || rec.Status != task.StatusQueued {
		t.Fatalf("recovered task status: %+v, err=%v", rec, err)
	}

	job, err = svc.Queue().Claim(ctx, []tasks.Kind{tasks.KindScan})
	if err != nil || job == nil {
		t.Fatalf("re-claim: %+v, err=%v", job, err)
	}

	expectedErr := errors.New("scan failure")
	if err := svc.Queue().Finish(ctx, job.ID, expectedErr); err != nil {
		t.Fatalf("finish: %v", err)
	}

	if !hookCalled.Load() {
		t.Fatal("finished hook was not called on Finish")
	}
	if hookResult == nil || hookResult.Error() != expectedErr.Error() {
		t.Fatalf("unexpected hook result: %v", hookResult)
	}

	rec, err = store.Client.Task.Get(ctx, info.ID)
	if err != nil || rec.Status != task.StatusFailed || *rec.Error != expectedErr.Error() {
		t.Fatalf("task final record: %+v, err=%v", rec, err)
	}
}

func TestPoolWorkerExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	registry := tasks.NewRegistry()
	executed := make(chan int, 5)

	registry.Register(tasks.NewHandler(
		tasks.KindScan,
		func(ctx context.Context, job tasks.Job) error {
			executed <- job.ID
			return nil
		},
		nil,
	))

	svc := tasks.NewService(store.Client, registry)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := tasks.NewPool(svc.Queue(), svc.Bus(), registry, 1, logger)

	info, err := store.Client.Task.Create().SetType(string(tasks.KindScan)).SetPayload(json.RawMessage(`{}`)).Save(ctx)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	poolDone := make(chan error, 1)
	go func() {
		poolDone <- pool.Run(ctx)
	}()

	select {
	case id := <-executed:
		if id != info.ID {
			t.Fatalf("executed job id mismatch: got %d, want %d", id, info.ID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pool did not execute job within deadline")
	}

	for range 50 {
		rec, _ := store.Client.Task.Get(context.Background(), info.ID)
		if rec != nil && rec.Status == task.StatusDone {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-poolDone:
		if err != nil {
			t.Fatalf("pool error on stop: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pool did not stop on context cancellation")
	}
}
