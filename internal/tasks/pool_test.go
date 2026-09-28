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
}

func (m *mockPoolQueue) Recover(ctx context.Context, kinds []Kind) error {
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

type mockPoolBus struct {
	ch chan struct{}
}

func (b *mockPoolBus) Pending() <-chan struct{} {
	return b.ch
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
	bus := &mockPoolBus{ch: make(chan struct{})}
	registry := NewRegistry()
	registry.Register(NewHandler(KindScan, func(ctx context.Context, job Job) error {
		return nil
	}, nil))

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := NewPool(mq, bus, registry, 1, logger)

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
