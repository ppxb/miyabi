package app

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/monitor"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestTaskGroupsFoldChildCountsAndLatestState(t *testing.T) {
	fix := libraryFixture(t)
	parent := fix.Queued
	payload := fix.Payload
	ctx := t.Context()
	if err := fix.Tasks.Queue().Finish(ctx, parent.ID, nil); err != nil {
		t.Fatal(err)
	}
	var activeID int
	latest := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, child := range []struct {
		kind   string
		status task.Status
	}{
		{"scrape", task.StatusDone}, {"scrape", task.StatusDone}, {"scrape", task.StatusFailed},
		{"cover", task.StatusDone}, {"cover", task.StatusRunning},
	} {
		input, err := tasks.EncodePayload(scrape.MetadataPayload{Source: payload.Source, ScanTaskID: parent.ID})
		if err != nil {
			t.Fatal(err)
		}
		input, err = tasks.SetPayloadField(input, "document", strings.Repeat("fixture document ", 1000))
		if err != nil {
			t.Fatal(err)
		}
		builder := fix.DB.Task.Create().SetType(child.kind).SetStatus(child.status).
			SetPayload(input).SetUpdatedAt(latest.Add(time.Duration(i) * time.Second))
		if child.status == task.StatusFailed {
			builder.SetError("fixture metadata failure")
		}
		record, err := builder.Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		activeID = record.ID
	}
	infos, err := fix.Service.Workflows(ctx, []*ent.Task{fix.DB.Task.GetX(ctx, parent.ID)})
	if err != nil {
		t.Fatal(err)
	}
	info := infos[0]
	if info.Status != string(task.StatusRunning) || info.Scan.Stage != "artwork" || info.Scan.MetadataTotal != 3 || info.Scan.MetadataCompleted != 2 || info.Progress != 66 || info.Error == nil {
		t.Fatalf("workflow: %+v", info)
	}
	if !info.UpdatedAt.Equal(latest.Add(4 * time.Second)) {
		t.Fatalf("latest change: %v", info.UpdatedAt)
	}
	if err := fix.Tasks.Queue().Finish(ctx, activeID, errors.New("fixture cover failure")); err != nil {
		t.Fatal(err)
	}
	infos, err = fix.Service.Workflows(ctx, []*ent.Task{fix.DB.Task.GetX(ctx, parent.ID)})
	if err != nil {
		t.Fatal(err)
	}
	info = infos[0]
	if err != nil || info.Status != string(task.StatusFailed) || info.Scan.MetadataCompleted != 3 || info.Progress != 100 {
		t.Fatalf("finished workflow: %+v err=%v", info, err)
	}
}

func TestTaskListRetainsOlderActiveWorkflows(t *testing.T) {
	fix := libraryFixture(t)
	parent := fix.Queued
	payload := fix.Payload
	ctx := t.Context()
	if err := fix.Tasks.Queue().Finish(ctx, parent.ID, nil); err != nil {
		t.Fatal(err)
	}
	input, err := tasks.EncodePayload(scrape.MetadataPayload{Source: payload.Source, ScanTaskID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := fix.DB.Task.Create().SetType("scrape").SetPayload(input).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	encoded, err := tasks.EncodePayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	for range 21 {
		if err := fix.DB.Task.Create().SetType("scan").SetStatus(task.StatusDone).SetPayload(encoded).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}
	views := &taskViews{Service: fix.Tasks, database: fix.DB, library: fix.Service, monitor: monitor.New(fix.DB, nil, nil, fix.Tasks)}
	items, err := views.List(ctx)
	if err != nil || len(items) != 21 || items[0].ID != parent.ID || items[0].Status != string(task.StatusQueued) {
		t.Fatalf("active workflows: %+v err=%v", items, err)
	}
}

func TestTaskViewsMergeScanAndBatchActivity(t *testing.T) {
	fix := libraryFixture(t)
	ctx := t.Context()
	batchPayload := json.RawMessage(`{"ids":[1,2],"batch":{"total":2,"processed":1,"waiting":1}}`)
	oldBatch := fix.DB.Task.Create().SetType("subscription_batch").SetStatus(task.StatusRunning).SetProgress(50).SetPayload(batchPayload).SaveX(ctx)
	var completed []int
	for range 7 {
		row := fix.DB.Task.Create().SetType("subscription_batch").SetStatus(task.StatusDone).SetProgress(100).SetPayload(batchPayload).SaveX(ctx)
		completed = append(completed, row.ID)
	}
	queuedBatch := fix.DB.Task.Create().SetType("subscription_batch").SetPayload(batchPayload).SaveX(ctx)
	scanPayload, err := tasks.EncodePayload(fix.Payload)
	if err != nil {
		t.Fatal(err)
	}
	runningScan := fix.DB.Task.Create().SetType("scan").SetStatus(task.StatusRunning).SetPayload(scanPayload).SaveX(ctx)
	views := &taskViews{Service: fix.Tasks, database: fix.DB, library: fix.Service, monitor: monitor.New(fix.DB, nil, nil, fix.Tasks)}
	items, err := views.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{runningScan.ID, oldBatch.ID, queuedBatch.ID, fix.Queued.ID, completed[6], completed[5], completed[4], completed[3]}
	var ids []int
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	if !slices.Equal(ids, want) {
		t.Fatalf("merged task order=%v want=%v", ids, want)
	}
	if batch := items[1]; batch.Batch == nil || batch.Batch.Total != 2 || batch.Batch.Processed != 1 || batch.Progress != 50 {
		t.Fatalf("batch progress lost: %+v", batch)
	}
	if items[0].Source != fix.Payload.Source || items[0].Batch != nil {
		t.Fatalf("scan identity changed: %+v", items[0])
	}
}
