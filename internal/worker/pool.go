package worker

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/ppxb/miyabi/internal/service"
	"golang.org/x/sync/errgroup"
)

type TaskStore interface {
	Recover(context.Context, []string) error
	Claim(context.Context, []string) (*service.TaskJob, error)
	Finish(context.Context, int, error) error
	Pending() <-chan struct{}
}

type Handler func(context.Context, service.TaskJob) error

type Pool struct {
	store    TaskStore
	handlers map[string]Handler
	size     int
	logger   *slog.Logger
}

func NewPool(store TaskStore, handlers map[string]Handler, size int, logger *slog.Logger) *Pool {
	return &Pool{store: store, handlers: handlers, size: size, logger: logger}
}

func (pool *Pool) Run(ctx context.Context) error {
	types := make([]string, 0, len(pool.handlers))
	for name := range pool.handlers {
		types = append(types, name)
	}
	slices.Sort(types)
	if err := pool.store.Recover(ctx, types); err != nil {
		return err
	}
	group, ctx := errgroup.WithContext(ctx)
	for range pool.size {
		group.Go(func() error { return pool.runWorker(ctx, types) })
	}
	return group.Wait()
}

func (pool *Pool) runWorker(ctx context.Context, types []string) error {
	for ctx.Err() == nil {
		job, err := pool.store.Claim(ctx, types)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return err
		}
		if job == nil {
			select {
			case <-ctx.Done():
				return nil
			case <-pool.store.Pending():
				continue
			}
		}
		pool.logger.InfoContext(ctx, "task started", "id", job.ID, "type", job.Type)
		runError := pool.handlers[job.Type](ctx, *job)
		if ctx.Err() != nil {
			// Keep running state for startup recovery, rather than reporting a
			// service shutdown as a failed user task.
			return nil
		}
		if err := pool.store.Finish(ctx, job.ID, runError); err != nil {
			return fmt.Errorf("persist task result: %w", err)
		}
		if runError != nil {
			pool.logger.ErrorContext(ctx, "task failed", "id", job.ID, "type", job.Type, "error", runError)
		} else {
			pool.logger.InfoContext(ctx, "task completed", "id", job.ID, "type", job.Type)
		}
	}
	return nil
}
