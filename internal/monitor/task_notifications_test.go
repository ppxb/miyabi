package monitor

import (
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestBatchProgressDoesNotWakeWorkersButEnqueueAndRetryDo(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	taskSvc := tasks.NewService(store.Client, tasks.NewRegistry())
	service := &Service{database: store.Client, tasks: taskSvc}
	work, stop := taskSvc.SubscribePool()
	defer stop()
	updates, unsubscribe := taskSvc.Subscribe()
	defer unsubscribe()
	id, err := service.enqueueSubscriptionBatch(t.Context(), []int{1})
	if err != nil {
		t.Fatal(err)
	}
	if len(work) != 1 || len(updates) != 1 {
		t.Fatal("enqueue missed worker or UI notification")
	}
	<-work
	<-updates
	payload := batchPayload{IDs: []int{1}, FailedIDs: []int{1}, Batch: domain.SubscriptionBatch{Total: 1, Processed: 1, Failed: 1}}
	if err := service.saveSubscriptionBatch(t.Context(), id, payload); err != nil {
		t.Fatal(err)
	}
	if len(work) != 0 || len(updates) != 1 {
		t.Fatal("batch progress did not stay on the UI channel")
	}
	<-updates
	store.Client.Task.UpdateOneID(id).SetStatus(task.StatusFailed).ExecX(t.Context())
	if _, err := service.RetryTask(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if len(work) != 1 || len(updates) != 1 {
		t.Fatal("retry missed worker or UI notification")
	}
}
