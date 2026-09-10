package service

import (
	"errors"
	"testing"

	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/setting"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/pan"
)

func TestMovieStatesFollowDownloadThroughIndexingWithoutCatalogueRequests(t *testing.T) {
	offline, record, input, source := offlineFixture(t)
	ctx := t.Context()
	// No JavDB client: state reads must remain entirely local.
	discover := &DiscoverService{database: offline.database}
	identity := []MovieIdentity{{ID: input.JavDBID, Code: input.Code}}
	assertState := func(want MovieState, libraryID int) {
		t.Helper()
		states, err := discover.MovieStates(ctx, identity)
		if err != nil || len(states) != 1 || states[0].ID != input.JavDBID ||
			states[0].State != want || states[0].LibraryID != libraryID {
			t.Fatalf("want %s with library %d, states=%+v err=%v", want, libraryID, states, err)
		}
	}
	assertState(MovieSaving, 0)
	if err := offline.updateTask(ctx, record, pan.OfflineTask{Status: 2, FileID: "download-folder"}, offline.drive.snapshot()); err != nil {
		t.Fatal(err)
	}
	assertState(MovieProcessing, 0)
	download := offline.database.Task.GetX(ctx, record.ID)
	saved, err := decodeTaskPayload[offlinePayload](download.Payload)
	if err != nil {
		t.Fatal(err)
	}
	scan := offline.database.Task.GetX(ctx, saved.ScanTaskID)
	payload, err := decodeTaskPayload[scanPayload](scan.Payload)
	if err != nil {
		t.Fatal(err)
	}
	library := NewLibraryService(offline.database, offline.drive, offline.tasks, nil)
	video := fixtureVideo("video-1", input.Code+".mp4")
	for range 2 {
		if err := library.indexScanPage(ctx, scan.ID, "first-page", "/Movies/download-folder", []scanVideo{video}, &payload); err != nil {
			t.Fatal(err)
		}
	}
	indexed := offline.database.File.Query().Where(file.FileIDEQ(video.ID)).OnlyX(ctx)
	assertState(MovieInLibrary, *indexed.MovieID)
	activity, err := offline.Activity(ctx)
	if err != nil || len(activity.Tasks) != 1 || activity.Tasks[0].Phase != "in_library" ||
		!activity.Tasks[0].Processing || activity.Tasks[0].LibraryID != *indexed.MovieID {
		t.Fatalf("indexed download is not playable during the scan: %+v err=%v", activity, err)
	}
	saved, err = decodeTaskPayload[offlinePayload](offline.database.Task.GetX(ctx, record.ID).Payload)
	if err != nil || len(saved.FileIDs) != 1 || saved.FileIDs[0] != video.ID {
		t.Fatalf("committed files are missing or duplicated: %+v err=%v", saved, err)
	}
	// A failed metadata job does not revoke playback of an indexed video.
	metadata, err := encodeTaskPayload(metadataPayload{Source: source, ScanTaskID: scan.ID,
		MovieID: *indexed.MovieID, Code: input.Code, JavDBID: input.JavDBID})
	if err != nil {
		t.Fatal(err)
	}
	scrape := offline.database.Task.Create().SetType("scrape").SetPayload(metadata).SaveX(ctx)
	if err := offline.tasks.Finish(ctx, scan.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := offline.tasks.Finish(ctx, scrape.ID, errors.New("metadata unavailable")); err != nil {
		t.Fatal(err)
	}
	assertState(MovieInLibrary, *indexed.MovieID)
	activity, err = offline.Activity(ctx)
	if err != nil || len(activity.Tasks) != 1 || activity.Tasks[0].Phase != "in_library" ||
		activity.Tasks[0].Processing || activity.Tasks[0].Error == nil {
		t.Fatalf("metadata failure hid playback or completion: %+v err=%v", activity, err)
	}
	offline.database.File.DeleteOne(indexed).ExecX(ctx)
	assertState(MovieNotInLibrary, 0)
}

func TestMovieStatesScopePendingWorkToTheMountedAccountAndRoot(t *testing.T) {
	offline, record, input, source := offlineFixture(t)
	ctx := t.Context()
	discover := &DiscoverService{database: offline.database}
	// A completed download may await scan creation after its mount returns.
	input.FileID = "download-folder"
	encoded, err := encodeTaskPayload(input)
	if err != nil {
		t.Fatal(err)
	}
	offline.database.Task.UpdateOne(record).SetStatus(task.StatusDone).SetPayload(encoded).ExecX(ctx)
	for _, scenario := range []struct {
		name, accountID, directoryID string
		want                         MovieState
	}{
		{"current source", source.AccountID, source.Directory.ID, MovieProcessing},
		{"other account", "another-account", source.Directory.ID, MovieNotInLibrary},
		{"other root", source.AccountID, "another-root", MovieNotInLibrary},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if err := saveSetting(ctx, offline.database, panDirectorySetting, panLibraryDirectory{
				AccountID: scenario.accountID, PanLibraryDirectory: PanLibraryDirectory{ID: scenario.directoryID},
			}); err != nil {
				t.Fatal(err)
			}
			states, err := discover.MovieStates(ctx, []MovieIdentity{{ID: input.JavDBID, Code: input.Code}})
			if err != nil || len(states) != 1 || states[0].State != scenario.want || states[0].LibraryID != 0 {
				t.Fatalf("state crossed its source: %+v err=%v", states, err)
			}
		})
	}
	offline.database.Setting.Delete().Where(setting.Key(panDirectorySetting)).ExecX(ctx)
	states, err := discover.MovieStates(ctx, []MovieIdentity{{ID: input.JavDBID, Code: input.Code}})
	if err != nil || len(states) != 1 || states[0].State != MovieNotInLibrary {
		t.Fatalf("unmounted state: %+v err=%v", states, err)
	}
}

func TestOfflinePageFileTrackingRollsBackWithTheIndex(t *testing.T) {
	offline, record, input, source := offlineFixture(t)
	ctx := t.Context()
	library := NewLibraryService(offline.database, offline.drive, offline.tasks, nil)
	payload := scanPayload{Source: source, OfflineTaskID: record.ID, TargetID: "download-folder",
		Code: input.Code, JavDBID: input.JavDBID}
	// The missing parent fails the final progress write after file tracking.
	if err := library.indexScanPage(ctx, -1, "rolled-back", "/Movies/download-folder",
		[]scanVideo{fixtureVideo("video", input.Code+".mp4")}, &payload); err == nil {
		t.Fatal("page with a missing scan parent unexpectedly committed")
	}
	if offline.database.File.Query().CountX(ctx) != 0 {
		t.Fatal("file index escaped rollback")
	}
	saved, err := decodeTaskPayload[offlinePayload](offline.database.Task.GetX(ctx, record.ID).Payload)
	if err != nil || len(saved.FileIDs) != 0 {
		t.Fatalf("download file tracking escaped rollback: %+v err=%v", saved, err)
	}
}
