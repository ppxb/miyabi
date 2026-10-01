package tasks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

type mockPoolQueue struct {
	mu             sync.Mutex
	claimAttempts  int
	finishAttempts int
	claimFailures  int
	finishFailures int
	job            *Job
	jobs           []*Job
	finished       chan struct{}
	recover        func(context.Context) error
}

func (m *mockPoolQueue) NextRetry(context.Context, []Kind) (time.Time, error) {
	return time.Time{}, nil
}

func (m *mockPoolQueue) Recover(ctx context.Context, kinds []Kind) error {
	if m.recover != nil {
		return m.recover(ctx)
	}
	return nil
}

func (m *mockPoolQueue) Claim(ctx context.Context, kinds []Kind) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.claimAttempts++
	if m.claimAttempts <= m.claimFailures {
		return nil, errors.New("sqlite: database is locked (5)")
	}
	if len(m.jobs) > 0 {
		job := m.jobs[0]
		m.jobs = m.jobs[1:]
		return job, nil
	}
	job := m.job
	m.job = nil
	return job, nil
}

func (m *mockPoolQueue) Finish(ctx context.Context, id int, runErr error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finishAttempts++
	if m.finishAttempts <= m.finishFailures {
		return errors.New("sqlite: database is locked (5)")
	}
	close(m.finished)
	return nil
}

func TestPoolWorkerRetryOnDBError(t *testing.T) {
	prevDelay := poolRetryDelay
	poolRetryDelay = 5 * time.Millisecond
	defer func() { poolRetryDelay = prevDelay }()

	finished := make(chan struct{})
	mq := &mockPoolQueue{
		claimFailures:  2,
		finishFailures: 2,
		job:            &Job{ID: 100, Type: KindScan},
		finished:       finished,
	}
	bus := NewBus()
	registry := NewRegistry()
	registry.Register(NewHandler(KindScan, func(ctx context.Context, job Job) error {
		return nil
	}, nil))

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := NewPool(mq, bus, registry, []Kind{KindScan}, 1, logger)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- pool.Run(ctx)
	}()

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for task to finish")
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected nil error on pool shutdown, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pool to stop")
	}

	mq.mu.Lock()
	defer mq.mu.Unlock()
	if mq.claimAttempts < 3 {
		t.Fatalf("expected at least 3 claim attempts (2 failures + 1 success), got %d", mq.claimAttempts)
	}
	if mq.finishAttempts != 3 {
		t.Fatalf("expected 3 finish attempts (2 failures + 1 success), got %d", mq.finishAttempts)
	}
}

func TestPoolSubscribesBeforeRecoveryAndUnsubscribesOnExit(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		name := "failed recovery"
		if canceled {
			name = "shutdown during recovery"
		}
		t.Run(name, func(t *testing.T) {
			bus := NewBus()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			failure := errors.New("fixture recovery failure")
			queue := &mockPoolQueue{recover: func(context.Context) error {
				bus.mu.Lock()
				subscribed := len(bus.workers)
				bus.mu.Unlock()
				if subscribed != 1 {
					t.Fatalf("recovery began without subscribing: %d", subscribed)
				}
				if canceled {
					cancel()
					return ctx.Err()
				}
				return failure
			}}
			err := NewPool(queue, bus, NewRegistry(), []Kind{KindScan}, 1, slog.Default()).Run(ctx)
			if canceled && err != nil || !canceled && !errors.Is(err, failure) {
				t.Fatalf("pool exit = %v", err)
			}
			bus.mu.Lock()
			defer bus.mu.Unlock()
			if len(bus.workers) != 0 {
				t.Fatal("pool left its wake subscription registered")
			}
		})
	}
}

func TestBusSeparatesWorkerWakeupsFromUIUpdates(t *testing.T) {
	bus := NewBus()
	updates, unsubscribe := bus.Subscribe()
	defer unsubscribe()
	var workers []<-chan struct{}
	for range 3 {
		listener, unsubscribe := bus.SubscribePool()
		defer unsubscribe()
		workers = append(workers, listener)
	}
	for _, notify := range []func(){bus.NotifyUI, bus.NotifyLibraryChanged, bus.NotifyOfflineChanged, bus.NotifyMonitorChanged} {
		notify()
		if len(updates) != 1 {
			t.Fatal("UI subscriber missed an update")
		}
		for i, listener := range workers {
			if len(listener) != 0 {
				t.Fatalf("UI update woke worker %d", i)
			}
		}
	}
	<-updates
	version, revisions := bus.Version(), bus.Revisions()
	bus.WakePool()
	bus.WakePool()
	for i, listener := range workers {
		if len(listener) != 1 {
			t.Fatalf("worker %d did not receive one coalesced wakeup", i)
		}
	}
	if len(updates) != 0 || bus.Version() != version || bus.Revisions() != revisions {
		t.Fatal("worker wakeup invalidated UI state")
	}
}

func TestIdlePoolIgnoresProgressAndOneWakeStartsConcurrentWorkers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		bus := NewBus()
		queue := &mockPoolQueue{}
		registry := NewRegistry()
		started := make(chan int, 3)
		registry.Register(NewHandler(KindScan, func(ctx context.Context, job Job) error {
			started <- job.ID
			<-ctx.Done()
			return ctx.Err()
		}, nil))
		stopped := make(chan error, 1)
		go func() {
			stopped <- NewPool(queue, bus, registry, []Kind{KindScan}, 3, slog.New(slog.NewTextHandler(io.Discard, nil))).Run(ctx)
		}()
		synctest.Wait()
		for range 100 {
			bus.NotifyUI()
			bus.NotifyLibraryChanged()
			bus.NotifyOfflineChanged()
			bus.NotifyMonitorChanged()
		}
		synctest.Wait()
		queue.mu.Lock()
		attempts := queue.claimAttempts
		queue.jobs = []*Job{{ID: 1, Type: KindScan}, {ID: 2, Type: KindScan}, {ID: 3, Type: KindScan}}
		queue.mu.Unlock()
		if attempts != 3 {
			t.Fatalf("idle pool made %d claims, want only 3 startup claims", attempts)
		}
		bus.WakePool()
		synctest.Wait()
		if len(started) != 3 {
			t.Fatalf("one enqueue wake only started %d of 3 workers", len(started))
		}
		cancel()
		synctest.Wait()
		if err := <-stopped; err != nil {
			t.Fatal(err)
		}
	})
}
