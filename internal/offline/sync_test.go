package offline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/offlinedownload"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/library/scan"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestOfflineSyncContinuesPastIndividualFailures(t *testing.T) {
	service, first, input, _ := offlineFixture(t)
	ctx := t.Context()
	records := []*ent.OfflineDownload{first}
	for index := 1; index < 5; index++ {
		payload := input
		payload.Hash = fmt.Sprintf("download-%d", index)
		payload.InfoHash = payload.Hash

		records = append(records, createDownload(service.database, payload).SetStatus(offlinedownload.StatusRunning).SaveX(ctx))
	}
	fetched := 0
	stubOf(t, service.drive).offlineTasks = func(_ context.Context, _ string, page int) (pan.OfflinePage, error) {
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
	}
	err := service.Sync(ctx)
	if err == nil || !strings.Contains(err.Error(), "unknown offline status 99") {
		t.Fatalf("individual sync failure was lost: %v", err)
	}
	for index, record := range records {
		current := service.database.OfflineDownload.GetX(ctx, record.ID)
		want := []offlinedownload.Status{offlinedownload.StatusRunning, offlinedownload.StatusDone, offlinedownload.StatusDone, offlinedownload.StatusRunning, offlinedownload.StatusFailed}[index]
		if current.Status != want {
			t.Errorf("task %d status = %s, want %s", record.ID, current.Status, want)
		}
		if index == 1 || index == 2 {
			if current.ScanTaskID == 0 || current.Progress != 100 {
				t.Errorf("completed download did not queue its scan: %+v", current)
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
	service, record, _, _ := offlineFixture(t)
	pageError := errors.New("fixture page unavailable")
	stubOf(t, service.drive).offlineTasks = func(_ context.Context, _ string, page int) (pan.OfflinePage, error) {
		if page == 1 {
			return pan.OfflinePage{PageCount: 2}, nil
		}
		return pan.OfflinePage{}, pageError
	}
	if err := service.Sync(t.Context()); !errors.Is(err, pageError) {
		t.Fatalf("page failure = %v", err)
	}
	current := service.database.OfflineDownload.GetX(t.Context(), record.ID)
	if current.Status != offlinedownload.StatusRunning || current.Error != nil {
		t.Fatalf("incomplete listing failed an unseen task: %+v", current)
	}
}

func TestOfflineCompletionWaitsForLocationAcrossRestart(t *testing.T) {
	service, record, input, source := offlineFixture(t)
	ctx := t.Context()
	fileID := ""
	stubOf(t, service.drive).offlineTasks = func(context.Context, string, int) (pan.OfflinePage, error) {
		return pan.OfflinePage{PageCount: 1, Tasks: []pan.OfflineTask{
			{Hash: input.InfoHash, Status: 2, Progress: 100, FileID: fileID},
		}}, nil
	}
	if err := service.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	current := service.database.OfflineDownload.GetX(ctx, record.ID)
	if current.Status != offlinedownload.StatusDone || current.Progress != 100 {
		t.Fatalf("remote completion was not persisted: %+v", current)
	}
	notifications, cancel := service.tasks.Subscribe()
	defer cancel()
	before := service.tasks.Revisions().Offline
	if err := service.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	after := service.tasks.Revisions().Offline
	if after != before {
		t.Fatalf("unchanged pending completion bumped revisions: before=%d after=%d", before, after)
	}
	select {
	case <-notifications:
		t.Fatal("unchanged pending completion published an event")
	default:
	}
	activity, err := service.Activity(ctx)
	if err != nil || len(activity.Tasks) != 1 || activity.Tasks[0].Phase != "processing" ||
		!activity.Tasks[0].Processing || activity.Tasks[0].ScanTaskID != 0 {
		t.Fatalf("pending completion activity = %+v, %v", activity, err)
	}
	if after := service.tasks.Revisions().Offline; after != before {
		t.Fatalf("unchanged pending completion was announced again: %+v", after)
	}
	service = New(service.database, nil, service.drive, tasks.NewService(service.database, tasks.NewRegistry()), service.library, service.submitTimeout)
	activity, err = service.Activity(ctx)
	if err != nil || len(activity.Tasks) != 1 || activity.Tasks[0].Phase != "processing" ||
		!activity.Tasks[0].Processing || activity.Tasks[0].ScanTaskID != 0 {
		t.Fatalf("restart restored completed download as downloading: %+v, %v", activity, err)
	}
	sess, err := service.drive.OpenSource(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	// A stale running result must not revert a saved remote completion.
	if err := service.UpdateTask(ctx, sess, record, pan.OfflineTask{Status: 1, Progress: 40}); err != nil {
		t.Fatal(err)
	}
	fileID = "download-folder"
	if err := service.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	current = service.database.OfflineDownload.GetX(ctx, record.ID)
	saved := *current
	if saved.ScanTaskID == 0 || saved.FileID != fileID || saved.AwaitingLocation {
		t.Fatalf("available file location did not resume indexing: %+v", saved)
	}
	if err := service.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if count := service.database.Task.Query().Where(task.TypeEQ("scan")).CountX(ctx); count != 2 {
		t.Fatalf("resuming completion duplicated scans: %d", count)
	}
	film := service.database.Movie.Create().SetCode(input.Code).SetJavdbID(input.JavdbID).SaveX(ctx)
	service.database.File.Create().SetFileID("video").SetName("video.mp4").SetSize(1).
		SetAccountID(source.AccountID).SetRootID(source.Directory.ID).SetMovie(film).SaveX(ctx)
	saved.FileIds = []string{"video"}
	service.database.OfflineDownload.UpdateOneID(record.ID).SetFileIds(saved.FileIds).ExecX(ctx)
	if err := service.tasks.Queue().Finish(ctx, saved.ScanTaskID, nil); err != nil {
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
	sess, err := service.drive.OpenSource(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateTask(ctx, sess, record, pan.OfflineTask{Status: 2}); err != nil {
		t.Fatal(err)
	}
	fetched := 0
	stubOf(t, service.drive).offlineTasks = func(context.Context, string, int) (pan.OfflinePage, error) {
		fetched++
		return pan.OfflinePage{PageCount: 1}, nil
	}
	if err := service.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	activity, err := service.Activity(ctx)
	if err != nil || len(activity.Tasks) != 1 {
		t.Fatalf("read activity: %+v, %v", activity, err)
	}
	state := activity.Tasks[0]
	if state.Status != string(offlinedownload.StatusDone) || state.Processing || state.Error == nil {
		t.Fatalf("removed remote history kept an endless pending workflow: %+v", state)
	}
	service = New(service.database, nil, service.drive, tasks.NewService(service.database, tasks.NewRegistry()), service.library, service.submitTimeout)
	if err := service.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if fetched != 1 {
		t.Fatalf("restart resumed a terminal workflow: %d polls", fetched)
	}
}

func TestOfflineProgressPublishesChanges(t *testing.T) {
	service, record, _, source := offlineFixture(t)
	sess, err := service.drive.OpenSource(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	updates, unsubscribe := service.tasks.Subscribe()
	defer unsubscribe()
	before := service.tasks.Revisions()
	remote := pan.OfflineTask{Status: 1, Progress: 45}
	if err := service.UpdateTask(t.Context(), sess, record, remote); err != nil {
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
	if err := service.UpdateTask(t.Context(), sess, record, remote); err != nil {
		t.Fatal(err)
	}
	if after := service.tasks.Revisions(); after.Offline != before.Offline+1 {
		t.Fatalf("unchanged progress published another revision: %+v", after)
	}
}

func TestOfflinePageFileTrackingRollsBackWithTheIndex(t *testing.T) {
	service, record, input, source := offlineFixture(t)
	ctx := t.Context()
	payload := scan.Payload{Source: source, OfflineTaskID: record.ID, TargetID: "download-folder",
		Code: input.Code, JavDBID: input.JavdbID}
	video := scan.IdentifyVideo(pan.File{ID: "video", ParentID: "download-folder", Name: input.Code + ".mp4", Size: 1 << 30})
	// The missing parent fails the final progress write after file tracking.
	if err := scan.ProcessScanPage(ctx, service.database, -1, "rolled-back", "/Movies/download-folder",
		[]scan.Video{video}, &payload, nil, service.tasks); err == nil {
		t.Fatal("page with a missing scan parent unexpectedly committed")
	}
	if service.database.File.Query().CountX(ctx) != 0 {
		t.Fatal("file index escaped rollback")
	}
	saved := *service.database.OfflineDownload.GetX(ctx, record.ID)
	if len(saved.FileIds) != 0 {
		t.Fatalf("download file tracking escaped rollback: %+v", saved)
	}
}
