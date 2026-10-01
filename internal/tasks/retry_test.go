package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
)

func TestLargeDelayedBacklogDoesNotBlockReadyTasks(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	later := time.Now().Add(time.Hour)
	for start := 0; start < 10_000; start += 250 {
		batch := make([]*ent.TaskCreate, 0, 250)
		for i := start; i < start+250; i++ {
			batch = append(batch, store.Client.Task.Create().SetType(string(KindScrape)).SetResourceKey(fmt.Sprintf("movie:%d", i)).SetRetryAt(later).SetRetryCount(1))
		}
		if err := store.Client.Task.CreateBulk(batch...).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewService(store.Client, NewRegistry())
	for _, kind := range []Kind{KindScan, KindScrape} {
		ready := store.Client.Task.Create().SetType(string(kind)).SetResourceKey("new:" + string(kind)).SaveX(ctx)
		job, err := svc.Queue().Claim(ctx, []Kind{kind})
		if err != nil || job == nil || job.ID != ready.ID {
			t.Fatalf("large delayed queue blocked %s: %+v %v", kind, job, err)
		}
	}
	next, err := svc.Queue().NextRetry(ctx, []Kind{KindScrape})
	if err != nil || !next.Equal(later) {
		t.Fatalf("large queue lost deadline: %v %v", next, err)
	}
}

func TestPoolWakesForPersistedRetryWithoutAnotherEnqueue(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	registry := NewRegistry()
	started := make(chan int, 1)
	registry.Register(NewHandler(KindScan, func(context.Context, Job) error { started <- 1; return nil }, nil))
	svc := NewService(store.Client, registry)
	store.Client.Task.Create().SetType(string(KindScan)).SetRetryCount(1).SetRetryAt(time.Now().Add(150 * time.Millisecond)).ExecX(ctx)
	stopped := make(chan error, 1)
	go func() {
		stopped <- NewPool(svc.Queue(), svc, registry, []Kind{KindScan}, 1, slog.New(slog.NewTextHandler(io.Discard, nil))).Run(ctx)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		cancel()
		<-stopped
		t.Fatal("scheduled retry never woke")
	}
	cancel()
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
}

func TestDelayedRetryPersistsAndPreservesResourceOrder(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	registry := NewRegistry()
	hooks := 0
	registry.Register(NewHandler(KindScrape, nil, func(context.Context, *ent.Tx, Job, error) (Change, error) { hooks++; return ChangeLibrary, nil }).WithRetry(domain.RetryDelay))
	svc := NewService(store.Client, registry)
	body := json.RawMessage(`{"metadata_ready":true,"artwork":{"poster":"saved"}}`)
	first := store.Client.Task.Create().SetType(string(KindScrape)).SetResourceKey("movie:1").SetPayload(body).SetProgress(60).SaveX(ctx)
	second := store.Client.Task.Create().SetType(string(KindScrape)).SetResourceKey("movie:1").SaveX(ctx)
	other := store.Client.Task.Create().SetType(string(KindScrape)).SetResourceKey("movie:2").SaveX(ctx)
	claim := func(want int) {
		t.Helper()
		job, err := svc.Queue().Claim(ctx, []Kind{KindScrape})
		if err != nil || want == 0 && job != nil || want != 0 && (job == nil || job.ID != want) {
			t.Fatalf("claim=%+v %v want=%d", job, err, want)
		}
	}
	claim(first.ID)
	if err := svc.Queue().Finish(ctx, first.ID, &domain.HTTPError{StatusCode: 429, RetryAfter: time.Hour}); err != nil {
		t.Fatal(err)
	}
	retry := store.Client.Task.GetX(ctx, first.ID)
	if retry.Status != task.StatusQueued || retry.RetryCount != 1 || retry.RetryAt == nil || time.Until(*retry.RetryAt) < 59*time.Minute || retry.Progress != 60 || string(retry.Payload) != string(body) || hooks != 0 {
		t.Fatalf("retry lost checkpoint or finished early: %+v hooks=%d", retry, hooks)
	}
	claim(other.ID)
	if err := svc.Queue().Finish(ctx, other.ID, nil); err != nil {
		t.Fatal(err)
	}
	claim(0)
	// Recreating the queue must preserve scheduled attempts and their budget.
	svc = NewService(store.Client, registry)
	if err := svc.Queue().Recover(ctx, []Kind{KindScrape}); err != nil {
		t.Fatal(err)
	}
	next, err := svc.Queue().NextRetry(ctx, []Kind{KindScrape})
	if err != nil || !next.Equal(*retry.RetryAt) {
		t.Fatalf("next retry=%v %v", next, err)
	}
	for attempt := 1; attempt <= MaxRetries; attempt++ {
		store.Client.Task.UpdateOneID(first.ID).SetRetryAt(time.Now().Add(-time.Second)).ExecX(ctx)
		claim(first.ID)
		if err := svc.Queue().Finish(ctx, first.ID, context.DeadlineExceeded); err != nil {
			t.Fatal(err)
		}
	}
	failed := store.Client.Task.GetX(ctx, first.ID)
	if failed.Status != task.StatusFailed || failed.RetryCount != MaxRetries || failed.RetryAt != nil || hooks != 2 {
		t.Fatalf("unbounded or premature retry: %+v hooks=%d", failed, hooks)
	}
	claim(second.ID)
}

func TestPermanentErrorsAndSubscriptionBatchesAreNotRetried(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	registry := NewRegistry()
	for _, kind := range []Kind{KindScan, KindScrape} {
		registry.Register(NewHandler(kind, nil, nil).WithRetry(domain.RetryDelay))
	}
	svc := NewService(store.Client, registry)
	for _, tc := range []struct {
		kind Kind
		err  error
	}{
		{KindScan, domain.E(domain.KindConflict, "ambiguous", nil)},
		{KindScrape, &domain.HTTPError{StatusCode: 404}},
		{KindSubscriptionBatch, context.DeadlineExceeded},
	} {
		job := store.Client.Task.Create().SetType(string(tc.kind)).SetStatus(task.StatusRunning).SaveX(t.Context())
		if err := svc.Queue().Finish(t.Context(), job.ID, tc.err); err != nil {
			t.Fatal(err)
		}
		got := store.Client.Task.GetX(t.Context(), job.ID)
		if got.Status != task.StatusFailed || got.RetryCount != 0 || got.RetryAt != nil {
			t.Fatalf("permanent task retried: %+v", got)
		}
	}
}
