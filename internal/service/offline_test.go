package service

import (
	"errors"
	"testing"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/pan"
)

func offlineFixture(t *testing.T) (*OfflineService, *ent.Task, offlinePayload, LibrarySource) {
	t.Helper()
	library, _, scan := libraryFixture(t)
	drive := &PanService{directory: panLibraryDirectory{
		AccountID: scan.Source.AccountID, PanLibraryDirectory: scan.Source.Directory,
	}}
	service := NewOfflineService(library.database, nil, drive, library.tasks)
	input := offlinePayload{AccountID: scan.Source.AccountID, DirectoryID: scan.Source.Directory.ID,
		Code: "ABP-001", JavDBID: "fixture-movie", Hash: "fixture-hash", InfoHash: "fixture-hash"}
	encoded, err := encodeTaskPayload(input)
	if err != nil {
		t.Fatal(err)
	}
	record, err := library.database.Task.Create().SetType("offline").SetStatus(task.StatusRunning).SetPayload(encoded).Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return service, record, input, scan.Source
}

func TestOfflineProjectionUsesMovieIdentityAndExposesPlaybackID(t *testing.T) {
	for _, test := range []struct {
		name    string
		code    string
		javdbID string
		phase   string
	}{
		{"same id with new spelling", "OLD-001", "fixture-movie", "in_library"},
		{"pending exact code", "ABP-001", "", "in_library"},
		{"conflicting id", "ABP-001", "other-movie", "downloaded"},
		{"pending different code", "ABP-002", "", "downloaded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, record, input, source := offlineFixture(t)
			ctx := t.Context()
			builder := service.database.Movie.Create().SetCode(test.code)
			if test.javdbID != "" {
				builder.SetJavdbID(test.javdbID)
			}
			local := builder.SaveX(ctx)
			service.database.File.Create().SetFileID("video").SetName("video.mp4").SetSize(1).
				SetAccountID(source.AccountID).SetRootID(source.Directory.ID).SetMovie(local).SaveX(ctx)
			input.FileIDs = []string{"video"}
			encoded, err := encodeTaskPayload(input)
			if err != nil {
				t.Fatal(err)
			}
			record = service.database.Task.UpdateOne(record).SetStatus(task.StatusDone).SetPayload(encoded).SaveX(ctx)
			result, err := service.submission(ctx, record, &source)
			if err != nil || result.Phase != test.phase {
				t.Fatalf("wrong offline identity: %#v, %v", result, err)
			}
			wantID := 0
			if test.phase == "in_library" {
				wantID = local.ID
			}
			if result.LibraryID != wantID {
				t.Fatalf("playback ID = %d, want %d", result.LibraryID, wantID)
			}
		})
	}
}

func TestOfflineCompletionAndTargetedScanCommitTogether(t *testing.T) {
	service, record, input, source := offlineFixture(t)
	ctx := t.Context()
	rollback := errors.New("fixture rollback")
	err := ent.WithTx(ctx, service.database, func(tx *ent.Tx) error {
		if err := service.completeTask(ctx, tx, record, input, "download-folder"); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback: %v", err)
	}
	unchanged, err := service.database.Task.Get(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Status != task.StatusRunning {
		t.Fatal("completion escaped rollback")
	}
	if count, err := service.database.Task.Query().Where(task.TypeEQ("scan")).Count(ctx); err != nil || count != 1 {
		t.Fatalf("scan escaped rollback: count=%d err=%v", count, err)
	}
	if err := service.updateTask(ctx, record, pan.OfflineTask{Status: 2, FileID: "download-folder"}); err != nil {
		t.Fatal(err)
	}
	done, err := service.database.Task.Get(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := decodeTaskPayload[offlinePayload](done.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != task.StatusDone || saved.ScanTaskID == 0 {
		t.Fatalf("completion: %#v %#v", done, saved)
	}
	scan, err := service.database.Task.Get(ctx, saved.ScanTaskID)
	if err != nil {
		t.Fatal(err)
	}
	target, err := decodeTaskPayload[scanPayload](scan.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if target.TargetID != "download-folder" || target.OfflineTaskID != record.ID || target.Source.AccountID != source.AccountID || target.Source.Directory.ID != source.Directory.ID ||
		target.Code != saved.Code || target.JavDBID != saved.JavDBID {
		t.Fatalf("scan scope: %#v", target)
	}
	// A previously queued full scan must not swallow the completed download.
	if count, err := service.database.Task.Query().Where(task.TypeEQ("scan")).Count(ctx); err != nil || count != 2 {
		t.Fatalf("targeted scan count=%d err=%v", count, err)
	}
	if err := service.updateTask(ctx, done, pan.OfflineTask{Status: 2, FileID: "download-folder"}); err != nil {
		t.Fatal(err)
	}
	if count, err := service.database.Task.Query().Where(task.TypeEQ("scan")).Count(ctx); err != nil || count != 2 {
		t.Fatalf("completion duplicated scan: count=%d err=%v", count, err)
	}
}

func TestOfflineActionDependsOnCurrentFilesRatherThanDownloadHistory(t *testing.T) {
	service, record, _, source := offlineFixture(t)
	ctx := t.Context()
	if err := service.updateTask(ctx, record, pan.OfflineTask{Status: 2, FileID: "video-1"}); err != nil {
		t.Fatal(err)
	}
	record, err := service.database.Task.Get(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	input, err := decodeTaskPayload[offlinePayload](record.Payload)
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.submission(ctx, record, &source)
	if err != nil || state.Phase != "processing" {
		t.Fatalf("processing: %#v %v", state, err)
	}
	if err := service.tasks.Finish(ctx, input.ScanTaskID, nil); err != nil {
		t.Fatal(err)
	}
	record.Payload["file_ids"] = []string{"video-1"}
	state, err = service.submission(ctx, record, &source)
	if err != nil || state.Phase != "available" {
		t.Fatalf("history without file: %#v %v", state, err)
	}
	movie, err := service.database.Movie.Create().SetCode(input.Code).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.database.File.Create().SetFileID("video-1").SetName("ABP-001.mp4").SetSize(1).
		SetAccountID(source.AccountID).SetRootID(source.Directory.ID).SetMovieID(movie.ID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	state, err = service.submission(ctx, record, &source)
	if err != nil || state.Phase != "in_library" {
		t.Fatalf("indexed file: %#v %v", state, err)
	}
	other := source
	other.Directory.ID = "another-root"
	state, err = service.submission(ctx, record, &other)
	if err != nil || state.Phase != "available" {
		t.Fatalf("other root: %#v %v", state, err)
	}
	if _, err := service.database.File.Delete().Where(file.FileIDEQ("video-1")).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	state, err = service.submission(ctx, record, &source)
	if err != nil || state.Phase != "available" {
		t.Fatalf("deleted file: %#v %v", state, err)
	}
}

func TestCompletedOfflineTaskDefersScanForAnotherMount(t *testing.T) {
	service, record, input, _ := offlineFixture(t)
	service.drive.directory.ID = "another-root"
	if err := service.updateTask(t.Context(), record, pan.OfflineTask{Status: 2, FileID: "download-folder"}); err != nil {
		t.Fatal(err)
	}
	record, err := service.database.Task.Get(t.Context(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := decodeTaskPayload[offlinePayload](record.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ScanTaskID != 0 || saved.DirectoryID != input.DirectoryID || saved.FileID != "download-folder" {
		t.Fatalf("unexpected scan or changed source: %#v", saved)
	}
}

func TestOfflineActivityKeepsLatestTasksInCurrentSource(t *testing.T) {
	service, original, input, source := offlineFixture(t)
	ctx := t.Context()
	var latest int
	for _, change := range []func(*offlinePayload){
		func(payload *offlinePayload) { payload.Code = "ABP-002"; payload.JavDBID = "latest-movie" },
		func(payload *offlinePayload) { payload.DirectoryID = "another-root" },
		func(payload *offlinePayload) { payload.AccountID = "another-account" },
	} {
		payload := input
		change(&payload)
		encoded, err := encodeTaskPayload(payload)
		if err != nil {
			t.Fatal(err)
		}
		record, err := service.database.Task.Create().SetType("offline").SetStatus(task.StatusRunning).
			SetPayload(encoded).Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if payload.Code == "ABP-002" {
			latest = record.ID
		}
	}
	activity, err := service.Activity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if activity.Source == nil || *activity.Source != source || len(activity.Tasks) != 1 {
		t.Fatalf("activity scope: %+v", activity)
	}
	current := activity.Tasks[0]
	if current.TaskID != latest || current.TaskID == original.ID || current.Code != "ABP-002" ||
		current.JavDBID != "latest-movie" || current.AccountID != source.AccountID ||
		current.DirectoryID != source.Directory.ID || current.Phase != "downloading" {
		t.Fatalf("latest task: %+v", current)
	}
	if err := saveSetting(ctx, service.database, panDirectorySetting, panLibraryDirectory{
		AccountID: source.AccountID, PanLibraryDirectory: PanLibraryDirectory{ID: "empty-root"},
	}); err != nil {
		t.Fatal(err)
	}
	activity, err = service.Activity(ctx)
	if err != nil || activity.Source == nil || activity.Source.Directory.ID != "empty-root" || len(activity.Tasks) != 0 {
		t.Fatalf("changed source: %+v err=%v", activity, err)
	}
}

func TestOfflineActivityWaitsForArtworkAndRechecksPlayableFiles(t *testing.T) {
	service, download, input, source := offlineFixture(t)
	ctx := t.Context()
	if err := service.updateTask(ctx, download, pan.OfflineTask{Status: 2, FileID: "download-folder"}); err != nil {
		t.Fatal(err)
	}
	download, err := service.database.Task.Get(ctx, download.ID)
	if err != nil {
		t.Fatal(err)
	}
	input, err = decodeTaskPayload[offlinePayload](download.Payload)
	if err != nil {
		t.Fatal(err)
	}
	scan, err := service.tasks.Info(ctx, input.ScanTaskID)
	if err != nil || scan.OfflineTaskID != download.ID {
		t.Fatalf("download scan identity: %+v err=%v", scan, err)
	}
	film, err := service.database.Movie.Create().SetCode(input.Code).SetScrapeStatus(movie.ScrapeStatusDone).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"unmatched-video", "matched-video"} {
		create := service.database.File.Create().SetFileID(id).SetName(id + ".mp4").SetSize(1).
			SetAccountID(source.AccountID).SetRootID(source.Directory.ID)
		if id == "matched-video" {
			create.SetMovieID(film.ID)
		}
		if err := create.Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}
	input.FileIDs = []string{"unmatched-video", "matched-video"}
	encoded, err := encodeTaskPayload(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.database.Task.UpdateOneID(download.ID).SetPayload(encoded).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	metadata, err := encodeTaskPayload(metadataPayload{Source: source, ScanTaskID: input.ScanTaskID, MovieID: film.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.database.Task.Create().SetType("scrape").SetStatus(task.StatusDone).SetPayload(metadata).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	cover, err := service.database.Task.Create().SetType("cover").SetStatus(task.StatusRunning).SetPayload(metadata).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.tasks.Finish(ctx, input.ScanTaskID, nil); err != nil {
		t.Fatal(err)
	}
	assertPhase := func(want string) {
		t.Helper()
		activity, err := service.Activity(ctx)
		if err != nil || len(activity.Tasks) != 1 || activity.Tasks[0].Phase != want ||
			activity.Tasks[0].ScanTaskID != input.ScanTaskID {
			t.Fatalf("want %s, activity=%+v err=%v", want, activity, err)
		}
	}
	assertPhase("processing")
	if err := service.tasks.Finish(ctx, cover.ID, nil); err != nil {
		t.Fatal(err)
	}
	assertPhase("in_library")
	if _, err := service.database.File.Delete().Where(file.FileIDEQ("matched-video")).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	assertPhase("downloaded")
	if _, err := service.database.File.Delete().Where(file.FileIDEQ("unmatched-video")).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	assertPhase("available")
}
