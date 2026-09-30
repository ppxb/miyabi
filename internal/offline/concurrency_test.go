package offline

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/offlinedownload"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/library/scan"
	"github.com/ppxb/miyabi/internal/pan"
)

type offlineAddResult struct {
	submission domain.OfflineSubmission
	err        error
}

func asyncOfflineAdd(ctx context.Context, service *Service, hash string) <-chan offlineAddResult {
	result := make(chan offlineAddResult, 1)
	go func() {
		submission, err := service.Add(ctx, "fixture-movie", hash)
		result <- offlineAddResult{submission, err}
	}()
	return result
}

func TestOfflineAddRejectsUnknownMagnetBeforeSubmission(t *testing.T) {
	service, client := offlineAddFixture(t)
	adds := 0
	client.addOffline = func(context.Context, string, string, string) (string, error) {
		adds++
		return "", nil
	}
	_, err := service.Add(t.Context(), "fixture-movie", strings.Repeat("c", 40))
	if !errors.Is(err, ErrMagnetNotFound) || adds != 0 || service.database.OfflineDownload.Query().CountX(t.Context()) != 0 {
		t.Fatalf("unknown magnet was submitted or recorded: adds=%d error=%v", adds, err)
	}
}

func TestOfflineConcurrentAddsDeduplicateWithoutBlockingOtherHashes(t *testing.T) {
	service, client := offlineAddFixture(t)
	started := make(chan struct{}, 1)
	hold, release := panTestGate(t)
	var addsA, addsB atomic.Int32
	client.addOffline = func(_ context.Context, _, uri, _ string) (string, error) {
		hash := strings.TrimPrefix(uri, "magnet:?xt=urn:btih:")
		if hash == offlineHashA {
			if addsA.Add(1) == 1 {
				started <- struct{}{}
			}
			<-hold
		} else {
			addsB.Add(1)
		}
		return hash, nil
	}
	first := asyncOfflineAdd(t.Context(), service, offlineHashA)
	awaitPan(t, started)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	canceled := asyncOfflineAdd(ctx, service, offlineHashA)
	duplicate := asyncOfflineAdd(t.Context(), service, strings.ToUpper(offlineHashA))
	awaitPanCondition(t, func() bool {
		service.operations.mu.Lock()
		defer service.operations.mu.Unlock()
		entry := service.operations.entries["100:"+offlineHashA]
		return entry != nil && entry.users == 3
	})
	cancel()
	if result := awaitPan(t, canceled); !errors.Is(result.err, context.Canceled) {
		t.Fatalf("canceled duplicate = %+v", result)
	}
	other := awaitPan(t, asyncOfflineAdd(t.Context(), service, offlineHashB))
	if other.err != nil || other.submission.Hash != offlineHashB {
		t.Fatalf("unrelated magnet = %+v", other)
	}
	release()
	added, repeated := awaitPan(t, first), awaitPan(t, duplicate)
	if added.err != nil || repeated.err != nil || added.submission.TaskID != repeated.submission.TaskID {
		t.Fatalf("duplicate submissions: first=%+v repeated=%+v", added, repeated)
	}
	if addsA.Load() != 1 || addsB.Load() != 1 || service.database.OfflineDownload.Query().CountX(t.Context()) != 2 {
		t.Fatalf("remote add counts: first=%d second=%d", addsA.Load(), addsB.Load())
	}
	if len(service.operations.entries) != 0 {
		t.Fatal("completed and canceled operations retained per-magnet locks")
	}
}

func TestOfflineStartedSubmissionKeepsOriginalSourceAfterCancellation(t *testing.T) {
	for _, action := range []string{"disconnect", "directory"} {
		t.Run(action, func(t *testing.T) {
			service, client := offlineAddFixture(t)
			source := *service.drive.Source()
			started := make(chan context.Context, 1)
			hold, release := panTestGate(t)
			client.addOffline = func(ctx context.Context, _, _, directory string) (string, error) {
				started <- ctx
				<-hold
				if directory != source.Directory.ID {
					return "", fmt.Errorf("submission changed its target directory")
				}
				return offlineHashA, ctx.Err()
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			finished := asyncOfflineAdd(ctx, service, offlineHashA)
			upstream := awaitPan(t, started)
			cancel()
			if action == "disconnect" {
				if _, err := service.drive.Disconnect(t.Context()); err != nil {
					t.Fatal(err)
				}
			} else if err := service.drive.ClearDirectory(t.Context()); err != nil {
				t.Fatal(err)
			}
			if deadline, ok := upstream.Deadline(); !ok || time.Until(deadline) > 2*time.Minute || upstream.Err() != nil {
				t.Fatal("started submission did not keep its bounded persistence context")
			}
			release()
			result := awaitPan(t, finished)
			if result.err != nil {
				t.Fatal(result.err)
			}
			record := service.database.OfflineDownload.GetX(t.Context(), result.submission.TaskID)
			input := *record
			if input.AccountID != source.AccountID || input.DirectoryID != source.Directory.ID ||
				input.InfoHash != offlineHashA || record.Status != offlinedownload.StatusRunning || input.ScanTaskID != 0 {
				t.Fatalf("started submission was lost or moved: %+v %+v", record, input)
			}
		})
	}
}

func TestOfflineSyncRespectsSourceChangesDuringRemotePolling(t *testing.T) {
	for _, action := range []string{"directory", "disconnect"} {
		t.Run(action, func(t *testing.T) {
			service, record, input, source := offlineFixture(t)
			started := make(chan struct{}, 1)
			hold, release := panTestGate(t)
			client := stubOf(t, service.drive)
			client.list = func(context.Context, string, string, int, int) (pan.FilePage, error) {
				return pan.FilePage{Path: []pan.Directory{{ID: source.Directory.ID, Name: source.Directory.Name}}}, nil
			}
			client.offlineTasks = func(context.Context, string, int) (pan.OfflinePage, error) {
				started <- struct{}{}
				<-hold
				return pan.OfflinePage{PageCount: 1, Tasks: []pan.OfflineTask{{Hash: input.InfoHash, Status: 2, FileID: "download-folder"}}}, nil
			}
			finished := make(chan error, 1)
			go func() { finished <- service.Sync(t.Context()) }()
			awaitPan(t, started)
			if action == "disconnect" {
				if _, err := service.drive.Disconnect(t.Context()); err != nil {
					t.Fatal(err)
				}
			} else if err := service.drive.ClearDirectory(t.Context()); err != nil {
				t.Fatal(err)
			}
			release()
			err := awaitPan(t, finished)
			current := service.database.OfflineDownload.GetX(t.Context(), record.ID)
			if action == "disconnect" {
				if !errors.Is(err, pan.ErrUnauthorized) || current.Status != offlinedownload.StatusRunning {
					t.Fatalf("old account sync changed state: %+v, %v", current, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			saved := *current
			if current.Status != offlinedownload.StatusDone || saved.FileID != "download-folder" || saved.ScanTaskID != 0 {
				t.Fatalf("completion on replaced mount = %+v", saved)
			}
			if _, err := service.drive.SelectDirectory(t.Context(), source.Directory.ID); err != nil {
				t.Fatal(err)
			}
			if err := service.Sync(t.Context()); err != nil {
				t.Fatal(err)
			}
			current = service.database.OfflineDownload.GetX(t.Context(), record.ID)
			saved = *current
			if saved.ScanTaskID == 0 || service.database.Task.Query().Where(task.TypeEQ("scan")).CountX(t.Context()) != 2 {
				t.Fatalf("remount did not resume targeted scan: %+v", saved)
			}
		})
	}
}

func TestOfflineStaleResultsPreserveCompletionAndIndexedFiles(t *testing.T) {
	service, record, _, source := offlineFixture(t)
	sess, err := service.drive.OpenSource(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	completed := pan.OfflineTask{Status: 2, FileID: "download-folder"}
	finished := make(chan error, 2)
	for range 2 {
		go func() { finished <- service.UpdateTask(t.Context(), sess, record, completed) }()
	}
	for range 2 {
		if err := awaitPan(t, finished); err != nil {
			t.Fatal(err)
		}
	}
	current := service.database.OfflineDownload.GetX(t.Context(), record.ID)
	saved := *current

	saved.FileIds = []string{"newly-indexed-video"}
	service.database.OfflineDownload.UpdateOneID(record.ID).SetFileIds(saved.FileIds).ExecX(t.Context())
	for _, remote := range []pan.OfflineTask{{Status: 1, Progress: 20}, {Status: -1}, {Status: 2, FileID: "old-download-folder"}} {
		if err := service.UpdateTask(t.Context(), sess, record, remote); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.markMissing(t.Context(), sess, record); err != nil {
		t.Fatal(err)
	}
	current = service.database.OfflineDownload.GetX(t.Context(), record.ID)
	latest := *current
	if current.Status != offlinedownload.StatusDone || current.Progress != 100 || current.Error != nil ||
		latest.ScanTaskID != saved.ScanTaskID || latest.FileID != saved.FileID || !slices.Equal(latest.FileIds, saved.FileIds) {
		t.Fatalf("stale result changed completion: %+v %+v", current, latest)
	}
	if count := service.database.Task.Query().Where(task.TypeEQ("scan")).CountX(t.Context()); count != 2 {
		t.Fatalf("concurrent completion created duplicate targeted scans: %d", count)
	}
}

func TestOfflineDuplicateHistoryPreservesRedownloadBehavior(t *testing.T) {
	for _, present := range []bool{false, true} {
		t.Run(fmt.Sprintf("video_present_%t", present), func(t *testing.T) {
			service, client := offlineAddFixture(t)
			adds, removes := 0, 0
			client.addOffline = func(context.Context, string, string, string) (string, error) {
				adds++
				if adds == 1 {
					return "", pan.ErrOfflineExists
				}
				return offlineHashA, nil
			}
			client.offlineTasks = func(context.Context, string, int) (pan.OfflinePage, error) {
				return pan.OfflinePage{PageCount: 1, Tasks: []pan.OfflineTask{{Hash: offlineHashA, Status: 2, FileID: "old-video", DirectoryID: "10"}}}, nil
			}
			client.info = func(context.Context, string, string) (pan.FileInfo, error) {
				if !present {
					return pan.FileInfo{}, pan.ErrNotFound
				}
				return pan.FileInfo{File: pan.File{ID: "old-video", ParentID: "10", Name: "ABP-001.mp4"}, Path: []pan.Directory{{ID: "10"}}}, nil
			}
			client.removeOffline = func(context.Context, string, string) error { removes++; return nil }
			result, err := service.Add(t.Context(), "fixture-movie", offlineHashA)
			if err != nil {
				t.Fatal(err)
			}
			if present {
				if adds != 1 || removes != 0 || result.Status != string(offlinedownload.StatusDone) || result.ScanTaskID == 0 {
					t.Fatalf("existing video was resubmitted: adds=%d removes=%d result=%+v", adds, removes, result)
				}
			} else if adds != 2 || removes != 1 || result.Status != string(offlinedownload.StatusRunning) || result.ScanTaskID != 0 {
				t.Fatalf("missing video was not resubmitted: adds=%d removes=%d result=%+v", adds, removes, result)
			}
		})
	}
}

func TestOfflinePlayableProcessingStillDeduplicatesUntilWorkflowFinishes(t *testing.T) {
	service, client := offlineAddFixture(t)
	ctx := t.Context()
	source := *service.drive.Source()
	sess, err := service.drive.OpenSource(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	input := ent.OfflineDownload{AccountID: source.AccountID, DirectoryID: source.Directory.ID,
		Code: "ABP-001", JavdbID: "fixture-movie", Hash: offlineHashA, InfoHash: offlineHashA}

	record := createDownload(service.database, input).SetStatus(offlinedownload.StatusRunning).SaveX(ctx)
	if err := service.UpdateTask(ctx, sess, record, pan.OfflineTask{Status: 2, FileID: "download-folder"}); err != nil {
		t.Fatal(err)
	}
	record = service.database.OfflineDownload.GetX(ctx, record.ID)
	input = *record

	film := service.database.Movie.Create().SetCode(input.Code).SetJavdbID(input.JavdbID).SaveX(ctx)
	video := service.database.File.Create().SetFileID("video").SetName("ABP-001.mp4").SetSize(1).
		SetAccountID(source.AccountID).SetRootID(source.Directory.ID).SetMovie(film).SaveX(ctx)
	record.FileIds = []string{video.FileID}
	service.database.OfflineDownload.UpdateOne(record).SetFileIds(record.FileIds).ExecX(ctx)
	adds := 0
	client.addOffline = func(context.Context, string, string, string) (string, error) {
		adds++
		return offlineHashA, nil
	}
	result, err := service.Add(ctx, input.JavdbID, input.Hash)
	if err != nil || result.TaskID != record.ID || result.Phase != "in_library" || !result.Processing || adds != 0 {
		t.Fatalf("playable processing download was resubmitted: %+v adds=%d err=%v", result, adds, err)
	}
	if err := service.tasks.Queue().Finish(ctx, input.ScanTaskID, nil); err != nil {
		t.Fatal(err)
	}
	service.database.File.DeleteOne(video).ExecX(ctx)
	result, err = service.Add(ctx, input.JavdbID, input.Hash)
	if err != nil || result.TaskID == record.ID || result.Phase != "downloading" || adds != 1 {
		t.Fatalf("deleted video could not be downloaded again: %+v adds=%d err=%v", result, adds, err)
	}
}

func TestOfflineCompletionAndScanPagePreserveEachOthersFields(t *testing.T) {
	service, record, input, source := offlineFixture(t)
	ctx := t.Context()
	sess, err := service.drive.OpenSource(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	parent := service.database.Task.Query().Where(task.TypeEQ("scan")).OnlyX(ctx)
	payload := domain.ScanPayload{Source: source, OfflineTaskID: record.ID, TargetID: "download-folder",
		Code: input.Code, JavDBID: input.JavdbID}
	video := scan.IdentifyVideo(pan.File{ID: "video", ParentID: payload.TargetID, Name: input.Code + ".mp4", Size: 1 << 30})
	start := make(chan struct{})
	finished := make(chan error, 2)
	go func() {
		<-start
		finished <- service.UpdateTask(ctx, sess, record, pan.OfflineTask{Status: 2, FileID: payload.TargetID})
	}()
	go func() {
		<-start
		payload.ScanID = "concurrent-scan"
		finished <- scan.ProcessScanPage(ctx, service.database, parent.ID, "/Movies/download-folder",
			[]scan.Video{video}, &payload, nil, service.tasks)
	}()
	close(start)
	for range 2 {
		if err := awaitPan(t, finished); err != nil {
			t.Fatal(err)
		}
	}
	saved := service.database.OfflineDownload.GetX(ctx, record.ID)
	if saved.Status != offlinedownload.StatusDone || saved.Progress != 100 || saved.FileID != "download-folder" ||
		saved.ScanTaskID == 0 || !slices.Equal(saved.FileIds, []string{"video"}) {
		t.Fatalf("completion and indexing overwrote each other: %+v", saved)
	}
}
