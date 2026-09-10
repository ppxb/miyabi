package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
)

func TestOfflineHistoryLoadsOnlyLatestTasksAndRetainsOldDownloads(t *testing.T) {
	service, pending, input, _ := offlineFixture(t)
	ctx := t.Context()
	service.database.Task.UpdateOne(pending).SetProgress(17).ExecX(ctx)
	var latestID int
	for version := range 4 {
		var jobs []*ent.TaskCreate
		for number := range 50 {
			payload := input
			payload.Hash, payload.InfoHash = fmt.Sprintf("history-%d", number), fmt.Sprintf("history-%d", number)
			jobs = append(jobs, service.database.Task.Create().SetType("offline").SetStatus(task.StatusDone).
				SetProgress(100).SetPayload(taskPayloadJSON(t, payload)))
		}
		records := service.database.Task.CreateBulk(jobs...).SaveX(ctx)
		if version == 3 {
			latestID = records[len(records)-1].ID
		}
	}
	rowsRead := 0
	service.database.Task.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			value, err := next.Query(ctx, query)
			if records, ok := value.([]*ent.Task); ok {
				rowsRead += len(records)
			}
			return value, err
		})
	}))
	for _, read := range []func() ([]OfflineSubmission, error){
		func() ([]OfflineSubmission, error) {
			activity, err := service.Activity(ctx)
			return activity.Tasks, err
		},
		func() ([]OfflineSubmission, error) { return service.Tasks(ctx, input.JavDBID, input.AccountID) },
	} {
		rowsRead = 0
		records, err := read()
		if err != nil || len(records) != 51 || rowsRead != 51 || records[0].TaskID != latestID {
			t.Fatalf("history was hydrated or limited before grouping: %d records, %d rows, %v", len(records), rowsRead, err)
		}
		old := records[len(records)-1]
		if old.TaskID != pending.ID || old.Status != task.StatusRunning || old.Progress != 17 || old.Phase != "downloading" {
			t.Fatalf("old download disappeared behind completed history: %#v", old)
		}
	}
}

func TestOfflineHistoryScopesMovieAndAccountBeforeGrouping(t *testing.T) {
	service, original, input, _ := offlineFixture(t)
	for _, change := range []func(*offlinePayload){
		func(payload *offlinePayload) { payload.JavDBID = "other-movie" },
		func(payload *offlinePayload) { payload.AccountID = "other-account" },
	} {
		payload := input
		change(&payload)
		service.database.Task.Create().SetType("offline").SetStatus(task.StatusDone).
			SetPayload(taskPayloadJSON(t, payload)).ExecX(t.Context())
	}
	records, err := service.Tasks(t.Context(), input.JavDBID, input.AccountID)
	if err != nil || len(records) != 1 || records[0].TaskID != original.ID {
		t.Fatalf("newer foreign task hid the movie's download: %#v, %v", records, err)
	}
}

func TestOfflineHistoryDoesNotHideMalformedHashBehindValidString(t *testing.T) {
	for _, hash := range []any{nil, 42, map[string]any{}, []any{}} {
		t.Run(fmt.Sprintf("%T", hash), func(t *testing.T) {
			service, _, input, _ := offlineFixture(t)
			bad := map[string]any{"account_id": input.AccountID, "directory_id": input.DirectoryID,
				"javdb_id": input.JavDBID, "hash": hash}
			service.database.Task.Create().SetType("offline").SetPayload(taskPayloadJSON(t, bad)).ExecX(t.Context())
			input.Hash = string(taskPayloadJSON(t, hash))
			service.database.Task.Create().SetType("offline").SetPayload(taskPayloadJSON(t, input)).ExecX(t.Context())
			if _, err := service.Activity(t.Context()); err == nil {
				t.Fatal("SQL grouping hid a malformed task identity")
			}
		})
	}
}
