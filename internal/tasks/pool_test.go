package tasks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type mockPoolQueue struct {
	mu             sync.Mutex
	claimAttempts  int
	finishAttempts int
	claimFailures  int
	finishFailures int
	job            *Job
	finished       chan struct{}
	recover        func(context.Context) error
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
				subscribed := len(bus.subscribers)
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
			err := NewPool(queue, bus, NewRegistry(), []Kind{KindScan}, 1, nil).Run(ctx)
			if canceled && err != nil || !canceled && !errors.Is(err, failure) {
				t.Fatalf("pool exit = %v", err)
			}
			bus.mu.Lock()
			defer bus.mu.Unlock()
			if len(bus.subscribers) != 0 {
				t.Fatal("pool left its wake subscription registered")
			}
		})
	}
}

func TestBusBroadcastsWakeupsToEveryPoolAndObserver(t *testing.T) {
	bus := NewBus()
	var listeners []<-chan struct{}
	for range 3 {
		listener, unsubscribe := bus.Subscribe()
		defer unsubscribe()
		listeners = append(listeners, listener)
	}
	for _, notify := range []func(){bus.Notify, bus.NotifyLibraryChanged, bus.NotifyOfflineChanged, bus.NotifyMonitorChanged} {
		notify()
		notify() // Coalescing must not steal another listener's wakeup.
		for i, listener := range listeners {
			select {
			case <-listener:
			default:
				t.Fatalf("listener %d missed the wakeup", i)
			}
			select {
			case <-listener:
				t.Fatalf("listener %d did not coalesce notifications", i)
			default:
			}
		}
	}
}
