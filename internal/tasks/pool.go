package tasks

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"golang.org/x/sync/errgroup"
)

var poolRetryDelay = 200 * time.Millisecond

// PoolQueue defines the queue operations required by the worker Pool.
type PoolQueue interface {
	Recover(context.Context, []Kind) error
	Claim(context.Context, []Kind) (*Job, error)
	Finish(context.Context, int, error) error
	NextRetry(context.Context, []Kind) (time.Time, error)
}

// PoolBus defines the notification wake operations required by the worker Pool.
type PoolBus interface {
	SubscribePool() (<-chan struct{}, func())
}

// Pool coordinates concurrent background workers executing registered task handlers.
type Pool struct {
	queue    PoolQueue
	bus      PoolBus
	registry *Registry
	kinds    []Kind
	size     int
	logger   *slog.Logger
}

// NewPool initializes a worker pool that only recovers and claims the given kinds.
func NewPool(queue PoolQueue, bus PoolBus, registry *Registry, kinds []Kind, size int, logger *slog.Logger) *Pool {
	return &Pool{
		queue:    queue,
		bus:      bus,
		registry: registry,
		kinds:    slices.Clone(kinds),
		size:     size,
		logger:   logger,
	}
}

// Run starts the worker goroutines and waits for context cancellation.
func (pool *Pool) Run(ctx context.Context) error {
	// Subscribe before recovery and claiming so an enqueue cannot be missed.
	// Each worker gets a wakeup so a coalesced batch can fill the pool without
	// claims broadcasting another event to every pool.
	pending := make([]<-chan struct{}, pool.size)
	for i := range pending {
		updates, unsubscribe := pool.bus.SubscribePool()
		pending[i] = updates
		defer unsubscribe()
	}
	if err := pool.queue.Recover(ctx, pool.kinds); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	group, ctx := errgroup.WithContext(ctx)
	for _, updates := range pending {
		group.Go(func() error { return pool.runWorker(ctx, updates) })
	}
	return group.Wait()
}

func (pool *Pool) runWorker(ctx context.Context, pending <-chan struct{}) error {
	for ctx.Err() == nil {
		job, err := pool.queue.Claim(ctx, pool.kinds)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			pool.logger.ErrorContext(ctx, "failed to claim task, retrying", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(poolRetryDelay):
			}
			continue
		}
		if job == nil {
			next, err := pool.queue.NextRetry(ctx, pool.kinds)
			if err != nil {
				next = time.Now().Add(poolRetryDelay)
			}
			var timer *time.Timer
			var due <-chan time.Time
			if !next.IsZero() {
				timer = time.NewTimer(time.Until(next))
				due = timer.C
			}
			select {
			case <-ctx.Done():
			case <-pending:
			case <-due:
			}
			if timer != nil {
				timer.Stop()
			}
			continue
		}
		handler, ok := pool.registry.Get(job.Type)
		if !ok {
			return fmt.Errorf("no handler registered for task type %s", job.Type)
		}
		pool.logger.InfoContext(ctx, "task started", "task_id", job.ID, "type", string(job.Type))
		runError := handler.Handle(ctx, *job)
		if ctx.Err() != nil {
			// Keep running state for startup recovery, rather than reporting a
			// service shutdown as a failed user task.
			return nil
		}
		for ctx.Err() == nil {
			if err := pool.queue.Finish(ctx, job.ID, runError); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				pool.logger.ErrorContext(ctx, "failed to finish task, retrying", "task_id", job.ID, "type", string(job.Type), "error", err)
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(poolRetryDelay):
				}
				continue
			}
			break
		}
		if ctx.Err() != nil {
			return nil
		}
		if runError != nil {
			pool.logger.WarnContext(ctx, "task attempt failed", "task_id", job.ID, "type", string(job.Type), "error", runError)
		} else {
			pool.logger.InfoContext(ctx, "task completed", "task_id", job.ID, "type", string(job.Type))
		}
	}
	return nil
}
