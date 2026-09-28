package monitor

import (
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/subscription"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestEnqueueSingle(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	item, err := f.service.AddMovie(ctx, "m1", AddMovieOptions{AutoDownload: ptr(false)})
	if err != nil {
		t.Fatal(err)
	}

	waiting, err := f.service.EnqueueSingle(ctx, item.ID)
	if err != nil || waiting.Status != StatusWaiting || !waiting.AutoDownload || waiting.NextCheckAt == nil || !waiting.NextCheckAt.After(time.Now()) {
		t.Fatalf("without a magnet the subscription waits with auto-download on and a future check: %#v %v", waiting, err)
	}

	f.discover.magnets["m1"] = []domain.Magnet{{Hash: "abc123", Name: "MOCK-m1", HasSubtitle: true, HD: true, Size: 1 << 30}}
	added, err := f.service.EnqueueSingle(ctx, item.ID)
	if err != nil || added.Status != StatusAdded || added.Hash != "abc123" || added.TaskID == nil {
		t.Fatalf("with a magnet the subscription is submitted: %#v %v", added, err)
	}
	if again, err := f.service.EnqueueSingle(ctx, item.ID); err != nil || again.Status != StatusAdded || len(f.offline.submissions) != 1 {
		t.Fatalf("enqueueing an added subscription must not resubmit: %#v %v submissions=%d", again, err, len(f.offline.submissions))
	}

	actor, err := f.service.AddActor(ctx, "a1", AddActorOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.EnqueueSingle(ctx, actor.ID); !domain.IsKind(err, domain.KindInvalid) {
		t.Fatalf("enqueueing an actor subscription must be invalid, got %v", err)
	}
}

func TestBatchEnqueueTask(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	item1, _ := f.service.AddMovie(ctx, "batch-1", AddMovieOptions{})
	item2, _ := f.service.AddMovie(ctx, "batch-2", AddMovieOptions{})
	f.discover.magnets["batch-1"] = []domain.Magnet{{Hash: "hash1", Name: "batch-1", HasSubtitle: true, HD: true, Size: 1000}}

	if _, err := f.service.EnqueueBatch(ctx, BatchEnqueueRequest{}); !domain.IsKind(err, domain.KindInvalid) {
		t.Fatalf("an empty selection must be invalid, got %v", err)
	}
	taskID, err := f.service.EnqueueBatch(ctx, BatchEnqueueRequest{IDs: []int{item1.ID, item2.ID, 424242}})
	if err != nil {
		t.Fatalf("EnqueueBatch: %v", err)
	}
	row, err := f.client.Task.Get(ctx, taskID)
	if err != nil || row.Type != string(tasks.KindSubscriptionBatch) {
		t.Fatalf("batch task row: %#v %v", row, err)
	}

	if err := f.service.BatchHandler(ctx, tasks.Job{ID: row.ID, Type: tasks.KindSubscriptionBatch, Payload: row.Payload}); err != nil {
		t.Fatalf("BatchHandler: %v", err)
	}
	row = f.client.Task.GetX(ctx, taskID)
	payload, err := tasks.DecodePayload[tasks.SubscriptionBatchPayload](row.Payload)
	if err != nil {
		t.Fatal(err)
	}
	batch := payload.Batch
	if batch.Total != 3 || batch.Processed != 3 || batch.Submitted != 1 || batch.Waiting != 1 || batch.Failed != 1 || len(batch.Failures) != 1 || row.Progress != 100 {
		t.Fatalf("unexpected tally %+v progress=%d", batch, row.Progress)
	}
	if sub := f.client.Subscription.GetX(ctx, item1.ID); sub.Status != subscription.StatusAdded || sub.Hash != "hash1" {
		t.Fatalf("item1 must be added with hash1, got %s %s", sub.Status, sub.Hash)
	}
	if sub := f.client.Subscription.GetX(ctx, item2.ID); sub.Status != subscription.StatusWaiting || !sub.AutoDownload {
		t.Fatalf("item2 must keep waiting with auto-download on, got %s %v", sub.Status, sub.AutoDownload)
	}

	infos, err := f.tasks.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var listed *tasks.TaskInfo
	for i := range infos {
		if infos[i].ID == taskID {
			listed = &infos[i]
		}
	}
	if listed == nil || listed.Type != string(tasks.KindSubscriptionBatch) || listed.Batch == nil || listed.Batch.Total != 3 {
		t.Fatalf("batch task must appear in the task list with its tally, got %+v", listed)
	}

	all, err := f.service.EnqueueBatch(ctx, BatchEnqueueRequest{All: true})
	if err != nil || all == 0 {
		t.Fatalf("all pending subscriptions must be enqueueable: %d %v", all, err)
	}
	if allRow := f.client.Task.GetX(ctx, all); allRow.ID == taskID {
		t.Fatal("expected a second task")
	}
}

func TestBatchHandlerResume(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	item1, _ := f.service.AddMovie(ctx, "resume-1", AddMovieOptions{})
	item2, _ := f.service.AddMovie(ctx, "resume-2", AddMovieOptions{})
	f.discover.magnets["resume-2"] = []domain.Magnet{{Hash: "hash2", Name: "resume-2", HasSubtitle: true, HD: true, Size: 1000}}

	taskID, err := f.service.EnqueueBatch(ctx, BatchEnqueueRequest{IDs: []int{item1.ID, item2.ID}})
	if err != nil {
		t.Fatalf("EnqueueBatch: %v", err)
	}

	payload := tasks.SubscriptionBatchPayload{
		IDs: []int{item1.ID, item2.ID},
		Batch: domain.SubscriptionBatch{
			Total:     2,
			Processed: 1,
			Waiting:   1,
		},
	}
	encoded, err := tasks.EncodePayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.client.Task.UpdateOneID(taskID).SetPayload(encoded).SetProgress(50).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	if err := f.service.BatchHandler(ctx, tasks.Job{ID: taskID, Type: tasks.KindSubscriptionBatch, Payload: encoded}); err != nil {
		t.Fatalf("BatchHandler: %v", err)
	}

	row := f.client.Task.GetX(ctx, taskID)
	savedPayload, err := tasks.DecodePayload[tasks.SubscriptionBatchPayload](row.Payload)
	if err != nil {
		t.Fatal(err)
	}
	batch := savedPayload.Batch
	if batch.Total != 2 || batch.Processed != 2 || batch.Submitted != 1 || batch.Waiting != 1 || row.Progress != 100 {
		t.Fatalf("unexpected tally after resume %+v progress=%d", batch, row.Progress)
	}
	if sub := f.client.Subscription.GetX(ctx, item1.ID); sub.Status != subscription.StatusWaiting {
		t.Fatalf("item1 must remain untouched, got %s", sub.Status)
	}
	if sub := f.client.Subscription.GetX(ctx, item2.ID); sub.Status != subscription.StatusAdded || sub.Hash != "hash2" {
		t.Fatalf("item2 must be added, got %s %s", sub.Status, sub.Hash)
	}
}
