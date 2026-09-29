package tasks

import (
	"context"
	"fmt"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/syncx"
)

// Queue owns task claiming and completion. It knows nothing about what a
// task does; handlers report their side effects through their Finished callback.
type Queue struct {
	database *ent.Client
	registry *Registry
	bus      *Bus
	lock     syncx.ContextLock
}

// Lock serialises enqueue decisions that must observe a consistent queue.
func (q *Queue) Lock(ctx context.Context) error { return q.lock.Lock(ctx) }
func (q *Queue) Unlock()                        { q.lock.Unlock() }

// Recover returns interrupted running tasks to the queue after a restart.
func (q *Queue) Recover(ctx context.Context, kinds []Kind) error {
	if _, err := q.database.Task.Update().Where(
		task.TypeIn(kindStrings(kinds)...), task.StatusEQ(task.StatusRunning),
	).SetStatus(task.StatusQueued).SetProgress(0).ClearError().Save(ctx); err != nil {
		return fmt.Errorf("recover interrupted tasks: %w", err)
	}
	q.bus.Notify()
	return nil
}

// Claim marks the oldest queued task of the given kinds running. It returns
// nil when nothing is queued.
func (q *Queue) Claim(ctx context.Context, kinds []Kind) (*Job, error) {
	if err := q.lock.Lock(ctx); err != nil {
		return nil, err
	}
	defer q.lock.Unlock()
	for {
		record, err := q.database.Task.Query().Where(
			task.TypeIn(kindStrings(kinds)...), task.StatusEQ(task.StatusQueued),
		).Order(ent.Asc(task.FieldID)).First(ctx)
		if ent.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("find queued task: %w", err)
		}
		claimed, err := q.database.Task.Update().Where(
			task.IDEQ(record.ID), task.StatusEQ(task.StatusQueued),
		).SetStatus(task.StatusRunning).SetProgress(0).ClearError().Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("claim task %d: %w", record.ID, err)
		}
		if claimed == 0 {
			continue
		}
		q.bus.Notify()
		return jobOf(record), nil
	}
}

// Finish records the outcome and runs the handler's completion hook in the
// same transaction. Revisions reported by the hook are published after commit.
func (q *Queue) Finish(ctx context.Context, id int, runError error) error {
	var change Change
	if err := ent.WithTx(ctx, q.database, func(tx *ent.Tx) error {
		record, err := tx.Task.Get(ctx, id)
		if err != nil {
			return err
		}
		update := tx.Task.UpdateOneID(id)
		if runError != nil {
			update.SetStatus(task.StatusFailed).SetError(runError.Error())
		} else {
			update.SetStatus(task.StatusDone).SetProgress(100).ClearError()
		}
		if err := update.Exec(ctx); err != nil {
			return err
		}
		job := jobOf(record)
		if handler, ok := q.registry.Get(job.Type); ok && handler.Finished != nil {
			change, err = handler.Finished(ctx, tx, *job, runError)
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("finish task %d: %w", id, err)
	}
	q.bus.publish(change)
	return nil
}

func jobOf(record *ent.Task) *Job {
	return &Job{ID: record.ID, Type: Kind(record.Type), Payload: record.Payload}
}

func kindStrings(kinds []Kind) []string {
	types := make([]string, len(kinds))
	for i, k := range kinds {
		types[i] = string(k)
	}
	return types
}
