package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/pan"
)

func TestOfflineSyncContinuesPastIndividualFailures(t *testing.T) {
	service, first, input, source := offlineFixture(t)
	ctx := t.Context()
	records := []*ent.Task{first}
	for index := 1; index < 5; index++ {
		payload := input
		payload.Hash = fmt.Sprintf("download-%d", index)
		payload.InfoHash = payload.Hash
		encoded, err := encodeTaskPayload(payload)
		if err != nil {
			t.Fatal(err)
		}
		status := task.StatusQueued
		if index == 3 {
			status = task.StatusRunning
		}
		records = append(records, service.database.Task.Create().SetType("offline").
			SetStatus(status).SetPayload(encoded).SaveX(ctx))
	}
	service.drive.tokens = panTestTokens("sync")
	fetched := 0
	service.drive.client = &panStub{
		account: func(context.Context, string) (pan.Account, error) { return pan.Account{ID: source.AccountID}, nil },
		offlineTasks: func(_ context.Context, _ string, page int) (pan.OfflinePage, error) {
			fetched++
			if page == 1 {
				return pan.OfflinePage{PageCount: 2, Tasks: []pan.OfflineTask{
					{Hash: input.InfoHash, Status: 99},
					{Hash: "download-1", Status: 2, FileID: "first-folder"},
				}}, nil
			}
			return pan.OfflinePage{PageCount: 2, Tasks: []pan.OfflineTask{
				{Hash: "download-2", Status: 2, FileID: "second-folder"},
				{Hash: "download-3", Status: 1, Progress: 65},
			}}, nil
		},
	}
	err := service.Sync(ctx)
	if err == nil || !strings.Contains(err.Error(), "unknown offline status 99") {
		t.Fatalf("individual sync failure was lost: %v", err)
	}
	for index, record := range records {
		current := service.database.Task.GetX(ctx, record.ID)
		want := []task.Status{task.StatusRunning, task.StatusDone, task.StatusDone, task.StatusRunning, task.StatusFailed}[index]
		if current.Status != want {
			t.Errorf("task %d status = %s, want %s", record.ID, current.Status, want)
		}
		if index == 1 || index == 2 {
			payload, err := decodeTaskPayload[offlinePayload](current.Payload)
			if err != nil || payload.ScanTaskID == 0 || current.Progress != 100 {
				t.Errorf("completed download did not queue its scan: %+v, %v", current, err)
			}
		}
		if index == 3 && current.Progress != 65 {
			t.Errorf("later progress was not synchronized: %d", current.Progress)
		}
	}
	if fetched != 2 {
		t.Errorf("fetched %d pages, want 2", fetched)
	}
}

func TestOfflineSyncKeepsUnseenTasksWhenLaterPageFails(t *testing.T) {
	service, record, _, source := offlineFixture(t)
	service.drive.tokens = panTestTokens("sync")
	pageError := errors.New("fixture page unavailable")
	service.drive.client = &panStub{
		account: func(context.Context, string) (pan.Account, error) { return pan.Account{ID: source.AccountID}, nil },
		offlineTasks: func(_ context.Context, _ string, page int) (pan.OfflinePage, error) {
			if page == 1 {
				return pan.OfflinePage{PageCount: 2}, nil
			}
			return pan.OfflinePage{}, pageError
		},
	}
	if err := service.Sync(t.Context()); !errors.Is(err, pageError) {
		t.Fatalf("page failure = %v", err)
	}
	current := service.database.Task.GetX(t.Context(), record.ID)
	if current.Status != task.StatusRunning || current.Error != nil {
		t.Fatalf("incomplete listing failed an unseen task: %+v", current)
	}
}

func TestOfflineCompletionWaitsForLocationAcrossRestart(t *testing.T) {
	service, record, input, source := offlineFixture(t)
	ctx := t.Context()
	service.drive.tokens = panTestTokens("sync")
	fileID := ""
	service.drive.client = &panStub{
		account: func(context.Context, string) (pan.Account, error) { return pan.Account{ID: source.AccountID}, nil },
		offlineTasks: func(context.Context, string, int) (pan.OfflinePage, error) {
			return pan.OfflinePage{PageCount: 1, Tasks: []pan.OfflineTask{
				{Hash: input.InfoHash, Status: 2, Progress: 100, FileID: fileID},
			}}, nil
		},
	}
	if err := service.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	current := service.database.Task.GetX(ctx, record.ID)
	if current.Status != task.StatusDone || current.Progress != 100 {
		t.Fatalf("remote completion was not persisted: %+v", current)
	}
	before := service.tasks.Revisions()
	if err := service.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if after := service.tasks.Revisions(); after != before {
		t.Fatalf("unchanged pending completion was announced again: %+v", after)
	}
	service = NewOfflineService(service.database, nil, service.drive, NewTaskService(service.database))
	activity, err := service.Activity(ctx)
	if err != nil || len(activity.Tasks) != 1 || activity.Tasks[0].Phase != "processing" ||
		!activity.Tasks[0].Processing || activity.Tasks[0].ScanTaskID != 0 {
		t.Fatalf("restart restored completed download as downloading: %+v, %v", activity, err)
	}
	// A stale running result must not revert a saved remote completion.
	if err := service.updateTask(ctx, record, pan.OfflineTask{Status: 1, Progress: 40}, service.drive.snapshot()); err != nil {
		t.Fatal(err)
	}
	fileID = "download-folder"
	if err := service.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	current = service.database.Task.GetX(ctx, record.ID)
	saved, err := decodeTaskPayload[offlinePayload](current.Payload)
	if err != nil || saved.ScanTaskID == 0 || saved.FileID != fileID || saved.AwaitingLocation {
		t.Fatalf("available file location did not resume indexing: %+v, %v", saved, err)
	}
	if err := service.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if count := service.database.Task.Query().Where(task.TypeEQ("scan")).CountX(ctx); count != 2 {
		t.Fatalf("resuming completion duplicated scans: %d", count)
	}
	film := service.database.Movie.Create().SetCode(input.Code).SetJavdbID(input.JavDBID).SaveX(ctx)
	service.database.File.Create().SetFileID("video").SetName("video.mp4").SetSize(1).
		SetAccountID(source.AccountID).SetRootID(source.Directory.ID).SetMovie(film).SaveX(ctx)
	saved.FileIDs = []string{"video"}
	encoded, err := encodeTaskPayload(saved)
	if err != nil {
		t.Fatal(err)
	}
	service.database.Task.UpdateOneID(record.ID).SetPayload(encoded).ExecX(ctx)
	if err := service.tasks.Finish(ctx, saved.ScanTaskID, nil); err != nil {
		t.Fatal(err)
	}
	activity, err = service.Activity(ctx)
	if err != nil || len(activity.Tasks) != 1 || activity.Tasks[0].Phase != "in_library" || activity.Tasks[0].Processing {
		t.Fatalf("finished workflow remained active after reload: %+v, %v", activity, err)
	}
}

func TestOfflineMissingLocationStopsPendingWorkflow(t *testing.T) {
	service, record, _, source := offlineFixture(t)
	ctx := t.Context()
	if err := service.updateTask(ctx, record, pan.OfflineTask{Status: 2}, service.drive.snapshot()); err != nil {
		t.Fatal(err)
	}
	service.drive.tokens = panTestTokens("sync")
	fetched := 0
	service.drive.client = &panStub{
		account: func(context.Context, string) (pan.Account, error) { return pan.Account{ID: source.AccountID}, nil },
		offlineTasks: func(context.Context, string, int) (pan.OfflinePage, error) {
			fetched++
			return pan.OfflinePage{PageCount: 1}, nil
		},
	}
	if err := service.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	activity, err := service.Activity(ctx)
	if err != nil || len(activity.Tasks) != 1 {
		t.Fatalf("read activity: %+v, %v", activity, err)
	}
	state := activity.Tasks[0]
	if state.Status != task.StatusDone || state.Processing || state.Error == nil {
		t.Fatalf("removed remote history kept an endless pending workflow: %+v", state)
	}
	service = NewOfflineService(service.database, nil, service.drive, NewTaskService(service.database))
	if err := service.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if fetched != 1 {
		t.Fatalf("restart resumed a terminal workflow: %d polls", fetched)
	}
}

func TestOfflineProgressPublishesChanges(t *testing.T) {
	service, record, _, _ := offlineFixture(t)
	updates, unsubscribe := service.tasks.Subscribe()
	defer unsubscribe()
	before := service.tasks.Revisions()
	remote := pan.OfflineTask{Status: 1, Progress: 45}
	if err := service.updateTask(t.Context(), record, remote, service.drive.snapshot()); err != nil {
		t.Fatal(err)
	}
	if after := service.tasks.Revisions(); after.Offline != before.Offline+1 {
		t.Fatalf("new progress did not publish an offline revision: before=%+v after=%+v", before, after)
	}
	select {
	case <-updates:
	default:
		t.Fatal("new progress did not wake the event stream")
	}
	if err := service.updateTask(t.Context(), record, remote, service.drive.snapshot()); err != nil {
		t.Fatal(err)
	}
	if after := service.tasks.Revisions(); after.Offline != before.Offline+1 {
		t.Fatalf("unchanged progress published another revision: %+v", after)
	}
}
