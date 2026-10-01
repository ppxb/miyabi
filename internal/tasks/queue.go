package tasks

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/predicate"
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

const MaxRetries = 3

// A delayed attempt retains its place for the same resource, while unrelated
// jobs remain runnable. The database key also survives process restarts.
func resourceAvailable(s *sql.Selector) {
	blocked := sql.Table(task.Table).As("blocked")
	s.Where(sql.Or(sql.EQ(s.C(task.FieldResourceKey), ""), sql.NotExists(sql.Select(blocked.C(task.FieldID)).From(blocked).Where(sql.And(
		sql.ColumnsEQ(blocked.C(task.FieldResourceKey), s.C(task.FieldResourceKey)),
		sql.In(blocked.C(task.FieldStatus), task.StatusQueued, task.StatusRunning),
		sql.Or(sql.ColumnsLT(blocked.C(task.FieldID), s.C(task.FieldID)), sql.EQ(blocked.C(task.FieldStatus), task.StatusRunning)),
	)))))
}

func (q *Queue) NextRetry(ctx context.Context, kinds []Kind) (time.Time, error) {
	kinds, err := q.runnableKinds(ctx, kinds)
	if err != nil || len(kinds) == 0 {
		return time.Time{}, err
	}
	record, err := q.database.Task.Query().Where(task.TypeIn(kindStrings(kinds)...), task.StatusEQ(task.StatusQueued),
		task.RetryAtNotNil(), predicate.Task(resourceAvailable)).Select(task.FieldRetryAt).
		Order(ent.Asc(task.FieldRetryAt)).First(ctx)
	if ent.IsNotFound(err) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return *record.RetryAt, nil
}

// Lock serialises enqueue decisions that must observe a consistent queue.
func (q *Queue) Lock(ctx context.Context) error { return q.lock.Lock(ctx) }
func (q *Queue) Unlock()                        { q.lock.Unlock() }

// Recover returns interrupted running tasks to the queue after a restart.
func (q *Queue) Recover(ctx context.Context, kinds []Kind) error {
	if _, err := q.database.Task.Update().Where(
		task.TypeIn(kindStrings(kinds)...), task.StatusEQ(task.StatusRunning),
	).SetStatus(task.StatusQueued).ClearError().Save(ctx); err != nil {
		return fmt.Errorf("recover interrupted tasks: %w", err)
	}
	q.bus.NotifyUI()
	return nil
}

// Claim marks the oldest queued task of the given kinds running. It returns
// nil when nothing is queued.
func (q *Queue) Claim(ctx context.Context, kinds []Kind) (*Job, error) {
	if err := q.lock.Lock(ctx); err != nil {
		return nil, err
	}
	defer q.lock.Unlock()
	kinds, err := q.runnableKinds(ctx, kinds)
	if err != nil || len(kinds) == 0 {
		return nil, err
	}
	for {
		record, err := q.database.Task.Query().Where(
			task.TypeIn(kindStrings(kinds)...), task.StatusEQ(task.StatusQueued),
			task.Or(task.RetryAtIsNil(), task.RetryAtLTE(time.Now())), predicate.Task(resourceAvailable),
		).Order(ent.Asc(task.FieldID)).First(ctx)
		if ent.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("find queued task: %w", err)
		}
		claimed, err := q.database.Task.Update().Where(
			task.IDEQ(record.ID), task.StatusEQ(task.StatusQueued),
		).SetStatus(task.StatusRunning).ClearRetryAt().ClearError().Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("claim task %d: %w", record.ID, err)
		}
		if claimed == 0 {
			continue
		}
		q.bus.NotifyUI()
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
		if errors.Is(runError, ErrPaused) {
			return update.SetStatus(task.StatusQueued).ClearError().Exec(ctx)
		}
		handler, registered := q.registry.Get(Kind(record.Type))
		if registered && handler.Retry != nil {
			if wait, retry := handler.Retry(runError); retry && record.RetryCount < MaxRetries {
				// Backoff starts at 15s; jitter avoids a simultaneous retry burst.
				delay := (15 * time.Second) << record.RetryCount
				delay += time.Duration(rand.Int64N(int64(delay / 4)))
				return update.SetStatus(task.StatusQueued).AddRetryCount(1).
					SetRetryAt(time.Now().Add(max(wait, delay))).SetError(runError.Error()).Exec(ctx)
			}
		}
		update.ClearRetryAt()
		if runError != nil {
			update.SetStatus(task.StatusFailed).SetError(runError.Error())
		} else {
			update.SetStatus(task.StatusDone).SetProgress(100).ClearError()
		}
		if err := update.Exec(ctx); err != nil {
			return err
		}
		job := jobOf(record)
		if registered && handler.Finished != nil {
			change, err = handler.Finished(ctx, tx, *job, runError)
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("finish task %d: %w", id, err)
	}
	q.bus.publish(change)
	q.bus.WakePool()
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
