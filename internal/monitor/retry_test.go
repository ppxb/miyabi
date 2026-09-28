package monitor

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestBatchRetryRetainsFailuresBeyondDisplayLimit(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	var ids []int
	for id := 1; id <= batchFailureLimit+5; id++ {
		ids = append(ids, id)
	}
	body, err := tasks.EncodePayload(batchPayload{IDs: ids, FailedIDs: ids, Batch: domain.SubscriptionBatch{
		Total: len(ids), Processed: len(ids), Failed: len(ids), Failures: make([]domain.SubscriptionFailure, batchFailureLimit)}})
	if err != nil {
		t.Fatal(err)
	}
	record := f.client.Task.Create().SetType(string(tasks.KindSubscriptionBatch)).SetStatus(task.StatusDone).SetPayload(body).SaveX(ctx)
	if _, err := f.service.RetryTask(ctx, record.ID); err != nil {
		t.Fatal(err)
	}
	saved, err := tasks.DecodePayload[batchPayload](f.client.Task.GetX(ctx, record.ID).Payload)
	if err != nil || !slices.Equal(saved.IDs, ids) {
		t.Fatalf("retry was limited to displayed errors: %+v, %v", saved, err)
	}
}

func TestBatchCheckpointFailureStopsAndCanResumeWithoutDuplicateSubmission(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	var ids []int
	for _, movieID := range []string{"first", "second"} {
		item, err := f.service.AddMovie(ctx, movieID, AddMovieOptions{})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, item.ID)
		f.discover.magnets[movieID] = []domain.Magnet{{Hash: movieID, HD: true, HasSubtitle: true, Size: 1 << 30}}
	}
	id, err := f.service.EnqueueBatch(ctx, BatchEnqueueRequest{IDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("checkpoint write failed")
	fail := true
	f.client.Task.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if _, ok := m.(*ent.TaskMutation).Payload(); ok && fail {
				return nil, failure
			}
			return next.Mutate(ctx, m)
		})
	})
	record := f.client.Task.GetX(ctx, id)
	err = f.service.BatchHandler(ctx, tasks.Job{ID: id, Type: tasks.KindSubscriptionBatch, Payload: record.Payload})
	if !errors.Is(err, failure) || len(f.offline.submissions) != 1 {
		t.Fatalf("batch continued past lost checkpoint: %v, %+v", err, f.offline.submissions)
	}
	if err := f.tasks.Queue().Finish(ctx, id, err); err != nil {
		t.Fatal(err)
	}
	fail = false
	if _, err := f.service.RetryTask(ctx, id); err != nil {
		t.Fatal(err)
	}
	record = f.client.Task.GetX(ctx, id)
	if err := f.service.BatchHandler(ctx, tasks.Job{ID: id, Type: tasks.KindSubscriptionBatch, Payload: record.Payload}); err != nil {
		t.Fatal(err)
	}
	if len(f.offline.submissions) != 2 || f.offline.submissions[0].MovieID != "first" || f.offline.submissions[1].MovieID != "second" {
		t.Fatalf("resuming duplicated submissions: %+v", f.offline.submissions)
	}
}

func TestBatchRetryKeepsOnlyFailedAndUnprocessedSubscriptions(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	payload := batchPayload{IDs: []int{1, 2, 3, 4, 5}, FailedIDs: []int{3, 3},
		Batch: domain.SubscriptionBatch{Total: 5, Processed: 4, Submitted: 1, Waiting: 1, Failed: 2}}
	body, err := tasks.EncodePayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	record := f.client.Task.Create().SetType(string(tasks.KindSubscriptionBatch)).SetStatus(task.StatusFailed).SetPayload(body).SetError("stopped").SaveX(ctx)
	info, err := f.service.RetryTask(ctx, record.ID)
	if err != nil || info.ID != record.ID || info.Status != "queued" || info.Batch.Total != 2 || info.CanRetry {
		t.Fatalf("retry = %+v, %v", info, err)
	}
	saved, err := tasks.DecodePayload[batchPayload](f.client.Task.GetX(ctx, record.ID).Payload)
	if err != nil || !slices.Equal(saved.IDs, []int{3, 5}) || saved.Batch.Processed != 0 || len(saved.FailedIDs) != 0 {
		t.Fatalf("wrong retry subset: %+v, %v", saved, err)
	}
	if _, err := f.service.RetryTask(ctx, record.ID); !domain.IsKind(err, domain.KindConflict) {
		t.Fatalf("active batch retried: %v", err)
	}
}

func TestBatchPersistsFailureIDsAndRetriesSuccessfulSubset(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	item, err := f.service.AddMovie(ctx, "retry-movie", AddMovieOptions{})
	if err != nil {
		t.Fatal(err)
	}
	id, err := f.service.EnqueueBatch(ctx, BatchEnqueueRequest{IDs: []int{item.ID}})
	if err != nil {
		t.Fatal(err)
	}
	f.discover.magnetsErr = errors.New("temporary rate limit")
	run := func() {
		t.Helper()
		record := f.client.Task.GetX(ctx, id)
		if err := f.service.BatchHandler(ctx, tasks.Job{ID: id, Type: tasks.KindSubscriptionBatch, Payload: record.Payload}); err != nil {
			t.Fatal(err)
		}
		if err := f.tasks.Queue().Finish(ctx, id, nil); err != nil {
			t.Fatal(err)
		}
	}
	run()
	record := f.client.Task.GetX(ctx, id)
	payload, err := tasks.DecodePayload[batchPayload](record.Payload)
	if err != nil || !slices.Equal(payload.FailedIDs, []int{item.ID}) {
		t.Fatalf("lost failed IDs: %+v, %v", payload, err)
	}
	info, err := batchTaskInfo(record)
	if err != nil || !info.CanRetry {
		t.Fatalf("failed subset is not retryable: %+v, %v", info, err)
	}
	f.discover.magnetsErr = nil
	f.discover.magnets["retry-movie"] = []domain.Magnet{{Hash: "fixture", HD: true, HasSubtitle: true, Size: 1 << 30}}
	if _, err := f.service.RetryTask(ctx, id); err != nil {
		t.Fatal(err)
	}
	run()
	if len(f.offline.submissions) != 1 {
		t.Fatalf("retry submissions: %+v", f.offline.submissions)
	}
	if _, err := f.service.RetryTask(ctx, id); !domain.IsKind(err, domain.KindConflict) {
		t.Fatalf("successful batch retried: %v", err)
	}
}

func TestLegacyBatchDoesNotGuessMissingFailureIDs(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	body, err := tasks.EncodePayload(batchPayload{IDs: []int{1, 2}, Batch: domain.SubscriptionBatch{Total: 2, Processed: 2, Failed: 1, Waiting: 1}})
	if err != nil {
		t.Fatal(err)
	}
	record := f.client.Task.Create().SetType(string(tasks.KindSubscriptionBatch)).SetStatus(task.StatusDone).SetPayload(body).SaveX(ctx)
	info, err := batchTaskInfo(record)
	if err != nil || info.CanRetry {
		t.Fatalf("legacy batch offered unsafe retry: %+v, %v", info, err)
	}
	if _, err := f.service.RetryTask(ctx, record.ID); !domain.IsKind(err, domain.KindConflict) {
		t.Fatalf("legacy failure IDs were guessed: %v", err)
	}
	if got := f.client.Task.GetX(ctx, record.ID); string(got.Payload) != string(body) || got.Status != task.StatusDone {
		t.Fatal("legacy history changed")
	}
}
