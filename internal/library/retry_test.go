package library

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestRetryScanPreservesCheckpointAndSource(t *testing.T) {
	lib, parent, _ := libraryFixture(t)
	ctx := t.Context()
	record := lib.database.Task.GetX(ctx, parent.ID)
	checkpoint := json.RawMessage(`{"scan_id":"resume-marker","checkpoint":"[{\"id\":\"20\"}]","scan":{"stage":"scanning","files_scanned":7},"source":{"account_id":"100","directory":{"id":"10","path":"/Movies"}}}`)
	record.Update().SetPayload(checkpoint).SetStatus(task.StatusFailed).SetRetryCount(3).SetError("fixture failure").ExecX(ctx)
	info, err := lib.RetryTask(ctx, record.ID)
	if err != nil || info.ID != record.ID || info.Status != "queued" {
		t.Fatalf("retry = %+v, %v", info, err)
	}
	saved := lib.database.Task.GetX(ctx, record.ID)
	if string(saved.Payload) != string(checkpoint) || saved.Error != nil || saved.Progress != 0 || saved.RetryCount != 0 || saved.RetryAt != nil {
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

func TestWaitingRetriesRemainActiveWithoutCountingAsCompleted(t *testing.T) {
	lib, parent, _ := libraryFixture(t)
	ctx := t.Context()
	lib.database.Task.UpdateOneID(parent.ID).SetStatus(task.StatusDone).ExecX(ctx)
	body, _ := json.Marshal(map[string]any{"scan_task_id": parent.ID, "metadata_ready": true})
	lib.database.Task.Create().SetType("scrape").SetPayload(body).SetStatus(task.StatusDone).ExecX(ctx)
	lib.database.Task.Create().SetType("scrape").SetPayload(body).SetStatus(task.StatusFailed).SetError("invalid identity").ExecX(ctx)
	lib.database.Task.Create().SetType("scrape").SetPayload(body).SetStatus(task.StatusQueued).SetRetryCount(1).SetRetryAt(time.Now().Add(time.Minute)).ExecX(ctx)
	info, err := lib.ListTasks(ctx)
	if err != nil || len(info) != 1 {
		t.Fatalf("tasks=%+v %v", info, err)
	}
	got := info[0]
	if got.Status != "queued" || got.CanRetry || got.Scan.MetadataTotal != 3 || got.Scan.MetadataCompleted != 2 || got.Scan.MetadataRetrying != 1 || got.Scan.MetadataFailed != 1 {
		t.Fatalf("retry was counted as a terminal failure: %+v", got)
	}
}

func TestReusedTasksContributeProgressAndCanBeRetried(t *testing.T) {
	lib, parent, input := libraryFixture(t)
	ctx := t.Context()
	body, _ := tasks.EncodePayload(map[string]any{"scan_task_id": 999, "source": input.Source, "metadata_ready": true})
	shared := lib.database.Task.Create().SetType("scrape").SetPayload(body).SetRetryCount(2).SetRetryAt(time.Now().Add(time.Minute)).SaveX(ctx)
	input.ReusedTasks = []int{shared.ID}
	input.Scan.Stage = "done"
	encoded, _ := tasks.EncodePayload(input)
	lib.database.Task.UpdateOneID(parent.ID).SetPayload(encoded).SetStatus(task.StatusDone).ExecX(ctx)
	// Keep this dependent scan visible even when its ID is outside the history page.
	for range 21 {
		lib.database.Task.Create().SetType("scan").SetPayload(encoded).SetStatus(task.StatusDone).ExecX(ctx)
	}
	infos, err := lib.ListTasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, info := range infos {
		if info.ID == parent.ID {
			found = true
			if info.Scan.MetadataTotal != 1 || info.Scan.MetadataRetrying != 1 || info.Scan.MetadataCompleted != 0 || info.Status != "queued" {
				t.Fatalf("shared progress lost: %+v", info)
			}
		}
	}
	if !found {
		t.Fatal("active dependent scan fell out of history")
	}
	shared.Update().SetStatus(task.StatusFailed).ClearRetryAt().SetError("timeout").ExecX(ctx)
	info, err := lib.RetryTask(ctx, parent.ID)
	if err != nil || info.Status != "queued" || info.Scan.MetadataTotal != 1 {
		t.Fatalf("retry shared work: %+v %v", info, err)
	}
	got := lib.database.Task.GetX(ctx, shared.ID)
	if got.Status != task.StatusQueued || got.RetryCount != 0 || string(got.Payload) != string(body) {
		t.Fatalf("retry changed ownership or checkpoint: %+v", got)
	}
}

func TestLargeSharedWorkflowAvoidsSQLiteParameterLimit(t *testing.T) {
	lib, parent, input := libraryFixture(t)
	ctx := t.Context()
	body, _ := tasks.EncodePayload(map[string]any{"scan_task_id": 999, "source": input.Source})
	shared := lib.database.Task.Create().SetType("scrape").SetStatus(task.StatusFailed).SetPayload(body).SaveX(ctx)
	// Repeated references also verify that a dependency is counted only once.
	input.ReusedTasks = make([]int, 40_000)
	for i := range input.ReusedTasks {
		input.ReusedTasks[i] = shared.ID
	}
	encoded, _ := tasks.EncodePayload(input)
	lib.database.Task.UpdateOneID(parent.ID).SetStatus(task.StatusDone).SetPayload(encoded).ExecX(ctx)
	info, err := lib.RetryTask(ctx, parent.ID)
	if err != nil || info.Scan.MetadataTotal != 1 || info.Status != "queued" {
		t.Fatalf("large shared workflow: %+v %v", info, err)
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
		{"scrape", task.StatusDone}, {"scrape", task.StatusDone},
		{"scrape", task.StatusFailed}, {"scrape", task.StatusDone}, {"scrape", task.StatusFailed},
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
	if err != nil || len(infos) != 1 || infos[0].CanRetry || infos[0].Scan.MetadataTotal != 5 || infos[0].Scan.MetadataCompleted != 3 {
		t.Fatalf("retry projection = %+v, %v", infos, err)
	}
}
