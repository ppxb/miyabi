package library

import (
	"encoding/json"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
)

func TestRetryScanPreservesCheckpointAndSource(t *testing.T) {
	lib, parent, _ := libraryFixture(t)
	ctx := t.Context()
	record := lib.database.Task.GetX(ctx, parent.ID)
	checkpoint := json.RawMessage(`{"scan_id":"resume-marker","checkpoint":"[{\"id\":\"20\"}]","scan":{"stage":"scanning","files_scanned":7},"source":{"account_id":"100","directory":{"id":"10","path":"/Movies"}}}`)
	record.Update().SetPayload(checkpoint).SetStatus(task.StatusFailed).SetError("fixture failure").ExecX(ctx)
	info, err := lib.RetryTask(ctx, record.ID)
	if err != nil || info.ID != record.ID || info.Status != "queued" {
		t.Fatalf("retry = %+v, %v", info, err)
	}
	saved := lib.database.Task.GetX(ctx, record.ID)
	if string(saved.Payload) != string(checkpoint) || saved.Error != nil || saved.Progress != 0 {
		t.Fatalf("checkpoint changed: %+v", saved)
	}
	if _, err := lib.RetryTask(ctx, record.ID); !domain.IsKind(err, domain.KindConflict) {
		t.Fatalf("active scan was retried: %v", err)
	}
	record.Update().SetStatus(task.StatusFailed).ExecX(ctx)
	if err := lib.drive.ClearDirectory(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.RetryTask(ctx, record.ID); err == nil {
		t.Fatal("retried a scan on an unmounted source")
	}
	if lib.database.Task.GetX(ctx, record.ID).Status != task.StatusFailed {
		t.Fatal("source rejection changed task state")
	}
}

func TestRetryMetadataOnlyRequeuesFailedChildrenOnce(t *testing.T) {
	lib, parent, _ := libraryFixture(t)
	ctx := t.Context()
	lib.database.Task.UpdateOneID(parent.ID).SetStatus(task.StatusDone).ExecX(ctx)
	var children []*ent.Task
	for _, state := range []struct {
		kind   string
		status task.Status
	}{
		{"scrape", task.StatusDone}, {"cover", task.StatusDone},
		{"scrape", task.StatusFailed}, {"scrape", task.StatusDone}, {"cover", task.StatusFailed},
	} {
		body, err := json.Marshal(map[string]any{"scan_task_id": parent.ID, "document": map[string]string{"title": "saved document"}, "artwork": map[string]string{"poster": "cached"}})
		if err != nil {
			t.Fatal(err)
		}
		children = append(children, lib.database.Task.Create().SetType(state.kind).SetStatus(state.status).SetPayload(body).SaveX(ctx))
	}
	unrelated := lib.database.Task.Create().SetType("scrape").SetStatus(task.StatusFailed).SetPayload(json.RawMessage(`{"scan_task_id":999}`)).SaveX(ctx)
	finished := make(chan error, 2)
	for range 2 {
		go func() { _, err := lib.RetryTask(ctx, parent.ID); finished <- err }()
	}
	succeeded, rejected := 0, 0
	for range 2 {
		err := awaitPan(t, finished)
		if err == nil {
			succeeded++
		} else if domain.IsKind(err, domain.KindConflict) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("duplicate retry: success=%d rejected=%d", succeeded, rejected)
	}
	for _, original := range children {
		got := lib.database.Task.GetX(ctx, original.ID)
		want := original.Status
		if want == task.StatusFailed {
			want = task.StatusQueued
		}
		if got.Status != want || string(got.Payload) != string(original.Payload) {
			t.Fatalf("retry changed unrelated work or payload: %+v", got)
		}
	}
	if lib.database.Task.GetX(ctx, unrelated.ID).Status != task.StatusFailed || lib.database.Task.GetX(ctx, parent.ID).Status != task.StatusDone {
		t.Fatal("retry restarted the scan or another workflow")
	}
	infos, err := lib.ListTasks(ctx)
	if err != nil || len(infos) != 1 || infos[0].CanRetry || infos[0].Scan.MetadataTotal != 3 || infos[0].Scan.MetadataCompleted != 1 {
		t.Fatalf("retry projection = %+v, %v", infos, err)
	}
}
