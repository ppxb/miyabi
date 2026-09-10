package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/javdb"
	"github.com/ppxb/miyabi/internal/pan"
)

const (
	offlineHashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	offlineHashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func offlineAddFixture(t *testing.T) (*OfflineService, *panStub) {
	t.Helper()
	library, client := panConcurrencyFixture(t)
	discover := &DiscoverService{
		database: library.database, details: newResponseCache[javdb.MovieDetail](2, time.Hour),
		magnets: newResponseCache[[]javdb.Magnet](2, time.Hour),
	}
	if _, err := discover.details.get(t.Context(), "fixture-movie", func(context.Context) (javdb.MovieDetail, error) {
		return javdb.MovieDetail{Movie: javdb.Movie{ID: "fixture-movie", Code: "ABP-001"}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := discover.magnets.get(t.Context(), "fixture-movie", func(context.Context) ([]javdb.Magnet, error) {
		return []javdb.Magnet{{Hash: offlineHashA}, {Hash: offlineHashB}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	return NewOfflineService(library.database, discover, library.drive, library.tasks), client
}

type offlineAddResult struct {
	submission OfflineSubmission
	err        error
}

func asyncOfflineAdd(ctx context.Context, service *OfflineService, hash string) <-chan offlineAddResult {
	result := make(chan offlineAddResult, 1)
	go func() {
		submission, err := service.Add(ctx, "fixture-movie", hash)
		result <- offlineAddResult{submission, err}
	}()
	return result
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
	if addsA.Load() != 1 || addsB.Load() != 1 || service.database.Task.Query().Where(task.TypeEQ("offline")).CountX(t.Context()) != 2 {
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
			source := service.drive.snapshot().source()
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
			record := service.database.Task.GetX(t.Context(), result.submission.TaskID)
			input, err := decodeTaskPayload[offlinePayload](record.Payload)
			if err != nil || input.AccountID != source.AccountID || input.DirectoryID != source.Directory.ID ||
				input.InfoHash != offlineHashA || record.Status != task.StatusRunning || input.ScanTaskID != 0 {
				t.Fatalf("started submission was lost or moved: %+v %+v %v", record, input, err)
			}
		})
	}
}

func TestOfflineSyncRespectsSourceChangesDuringRemotePolling(t *testing.T) {
	for _, action := range []string{"directory", "disconnect"} {
		t.Run(action, func(t *testing.T) {
			service, record, input, source := offlineFixture(t)
			service.drive.database = service.database
			service.drive.tokens = panTestTokens("sync")
			started := make(chan struct{}, 1)
			hold, release := panTestGate(t)
			client := &panStub{
				account: func(context.Context, string) (pan.Account, error) { return pan.Account{ID: source.AccountID}, nil },
				list: func(context.Context, string, string, int, int) (pan.FilePage, error) {
					return pan.FilePage{Path: []pan.Directory{{ID: source.Directory.ID, Name: source.Directory.Name}}}, nil
				},
				offlineTasks: func(context.Context, string, int) (pan.OfflinePage, error) {
					started <- struct{}{}
					<-hold
					return pan.OfflinePage{PageCount: 1, Tasks: []pan.OfflineTask{{Hash: input.InfoHash, Status: 2, FileID: "download-folder"}}}, nil
				},
			}
			service.drive.client = client
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
			current := service.database.Task.GetX(t.Context(), record.ID)
			if action == "disconnect" {
				if !errors.Is(err, pan.ErrUnauthorized) || current.Status != task.StatusRunning {
					t.Fatalf("old account sync changed state: %+v, %v", current, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			saved, err := decodeTaskPayload[offlinePayload](current.Payload)
			if err != nil || current.Status != task.StatusDone || saved.FileID != "download-folder" || saved.ScanTaskID != 0 {
				t.Fatalf("completion on replaced mount = %+v, %v", saved, err)
			}
			if _, err := service.drive.SelectDirectory(t.Context(), source.Directory.ID); err != nil {
				t.Fatal(err)
			}
			if err := service.Sync(t.Context()); err != nil {
				t.Fatal(err)
			}
			current = service.database.Task.GetX(t.Context(), record.ID)
			saved, err = decodeTaskPayload[offlinePayload](current.Payload)
			if err != nil || saved.ScanTaskID == 0 || service.database.Task.Query().Where(task.TypeEQ("scan")).CountX(t.Context()) != 2 {
				t.Fatalf("remount did not resume targeted scan: %+v, %v", saved, err)
			}
		})
	}
}

func TestOfflineStaleResultsPreserveCompletionAndNewerPayload(t *testing.T) {
	service, record, _, _ := offlineFixture(t)
	state := service.drive.snapshot()
	completed := pan.OfflineTask{Status: 2, FileID: "download-folder"}
	finished := make(chan error, 2)
	for range 2 {
		go func() { finished <- service.updateTask(t.Context(), record, completed, state) }()
	}
	for range 2 {
		if err := awaitPan(t, finished); err != nil {
			t.Fatal(err)
		}
	}
	current := service.database.Task.GetX(t.Context(), record.ID)
	saved, err := decodeTaskPayload[offlinePayload](current.Payload)
	if err != nil {
		t.Fatal(err)
	}
	saved.FileIDs = []string{"newly-indexed-video"}
	encoded, err := encodeTaskPayload(saved)
	if err != nil {
		t.Fatal(err)
	}
	service.database.Task.UpdateOneID(record.ID).SetPayload(encoded).ExecX(t.Context())
	for _, remote := range []pan.OfflineTask{{Status: 1, Progress: 20}, {Status: -1}, {Status: 2, FileID: "old-download-folder"}} {
		if err := service.updateTask(t.Context(), record, remote, state); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.markMissing(t.Context(), record, state); err != nil {
		t.Fatal(err)
	}
	current = service.database.Task.GetX(t.Context(), record.ID)
	latest, err := decodeTaskPayload[offlinePayload](current.Payload)
	if err != nil || current.Status != task.StatusDone || current.Progress != 100 || current.Error != nil ||
		latest.ScanTaskID != saved.ScanTaskID || latest.FileID != saved.FileID || !slices.Equal(latest.FileIDs, saved.FileIDs) {
		t.Fatalf("stale result changed completion: %+v %+v, %v", current, latest, err)
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
				if adds != 1 || removes != 0 || result.Status != task.StatusDone || result.ScanTaskID == 0 {
					t.Fatalf("existing video was resubmitted: adds=%d removes=%d result=%+v", adds, removes, result)
				}
			} else if adds != 2 || removes != 1 || result.Status != task.StatusRunning || result.ScanTaskID != 0 {
				t.Fatalf("missing video was not resubmitted: adds=%d removes=%d result=%+v", adds, removes, result)
			}
		})
	}
}
