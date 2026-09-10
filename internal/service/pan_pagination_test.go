package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/pan"
)

func TestFilePaginationRejectsIncompletePagesBeforeVisitingThem(t *testing.T) {
	first := pan.FilePage{Total: 2, HasMore: true, Files: []pan.File{{ID: "first"}}}
	for _, last := range []pan.FilePage{
		{Total: 3, Files: []pan.File{{ID: "last"}}},
		{Total: 2, HasMore: true},
		{Total: 2},
		{Total: 2, Files: []pan.File{{ID: "last"}, {ID: "extra"}}},
	} {
		t.Run(fmt.Sprintf("total_%d_count_%d_more_%t", last.Total, len(last.Files), last.HasMore), func(t *testing.T) {
			visited, fetched := 0, 0
			err := walkFilePages(t.Context(), func(offset int) (pan.FilePage, error) {
				fetched++
				if offset == 0 {
					return first, nil
				}
				return last, nil
			}, func(pan.FilePage) (bool, error) { visited++; return true, nil })
			if !errors.Is(err, errPanDirectoryIncomplete) || visited != 1 || fetched != 2 {
				t.Fatalf("incomplete page was visited: fetched=%d visited=%d err=%v", fetched, visited, err)
			}
		})
	}
}

func TestFilePaginationStopsForResultsCancellationAndVisitorErrors(t *testing.T) {
	for _, action := range []string{"complete", "found", "cancel", "error"} {
		t.Run(action, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			failure := errors.New("cannot persist scanned page")
			fetched := 0
			err := walkFilePages(ctx, func(offset int) (pan.FilePage, error) {
				fetched++
				if offset != fetched-1 {
					t.Errorf("page offset=%d, want %d", offset, fetched-1)
				}
				return pan.FilePage{Total: 2, HasMore: offset == 0, Files: []pan.File{{ID: fmt.Sprint(offset)}}}, nil
			}, func(pan.FilePage) (bool, error) {
				switch action {
				case "found":
					return false, nil
				case "cancel":
					cancel()
				case "error":
					return false, failure
				}
				return true, nil
			})
			var want error
			wantPages := 1
			switch action {
			case "complete":
				wantPages = 2
			case "cancel":
				want = context.Canceled
			case "error":
				want = failure
			}
			if !errors.Is(err, want) || fetched != wantPages {
				t.Fatalf("fetches=%d err=%v, want %d and %v", fetched, err, wantPages, want)
			}
		})
	}
}

func TestScanIncompleteLaterPageDoesNotPruneExistingFiles(t *testing.T) {
	library, client := panConcurrencyFixture(t)
	ctx := t.Context()
	queued := library.database.Task.Query().Where(task.TypeEQ("scan")).OnlyX(ctx)
	payload := scanPayload{Source: library.drive.snapshot().source()}
	if err := library.indexScanPage(ctx, queued.ID, "previous", "/Movies", []scanVideo{
		fixtureVideo("101", "ABP-001.mp4"), fixtureVideo("102", "ABP-002.mp4"),
	}, &payload); err != nil {
		t.Fatal(err)
	}
	client.list = func(_ context.Context, _, _ string, offset, _ int) (pan.FilePage, error) {
		page := pan.FilePage{Total: 2, Path: []pan.Directory{{ID: "10"}}}
		if offset == 0 {
			page.Files, page.HasMore = []pan.File{fixtureVideo("101", "ABP-001.mp4").File}, true
		}
		return page, nil
	}
	queued = library.database.Task.GetX(ctx, queued.ID)
	err := library.Scan(ctx, TaskJob{ID: queued.ID, Type: "scan", Payload: queued.Payload})
	if !errors.Is(err, errPanDirectoryIncomplete) {
		t.Fatalf("incomplete scan = %v", err)
	}
	if library.database.File.Query().CountX(ctx) != 2 ||
		library.database.File.Query().Where(file.FileIDEQ("102")).OnlyX(ctx).ScanID != "previous" ||
		library.database.Task.Query().Where(task.TypeEQ("scrape")).ExistX(ctx) {
		t.Fatal("incomplete scan pruned unvisited files or scheduled metadata")
	}
}

func TestOfflineIncompleteOutputListingNeverRemovesHistory(t *testing.T) {
	service, client := offlineAddFixture(t)
	client.addOffline = func(context.Context, string, string, string) (string, error) { return "", pan.ErrOfflineExists }
	client.offlineTasks = func(context.Context, string, int) (pan.OfflinePage, error) {
		return pan.OfflinePage{PageCount: 1, Tasks: []pan.OfflineTask{{Hash: offlineHashA, Status: 2, FileID: "folder"}}}, nil
	}
	client.info = func(context.Context, string, string) (pan.FileInfo, error) {
		return pan.FileInfo{File: pan.File{ID: "folder", IsDirectory: true}, Path: []pan.Directory{{ID: "10"}}}, nil
	}
	client.list = func(context.Context, string, string, int, int) (pan.FilePage, error) {
		return pan.FilePage{Total: 1, Path: []pan.Directory{{ID: "10"}}}, nil
	}
	removed := false
	client.removeOffline = func(context.Context, string, string) error { removed = true; return nil }
	_, err := service.Add(t.Context(), "fixture-movie", offlineHashA)
	if !errors.Is(err, errPanDirectoryIncomplete) || removed {
		t.Fatalf("uncertain download output: removed=%t err=%v", removed, err)
	}
}

func TestOfflineLookupAndSyncStopAfterFindingWantedTasks(t *testing.T) {
	for _, action := range []string{"lookup", "sync"} {
		t.Run(action, func(t *testing.T) {
			service, record, input, source := offlineFixture(t)
			service.drive.tokens = panTestTokens("pagination")
			fetched := 0
			service.drive.client = &panStub{
				account: func(context.Context, string) (pan.Account, error) { return pan.Account{ID: source.AccountID}, nil },
				offlineTasks: func(_ context.Context, _ string, page int) (pan.OfflinePage, error) {
					fetched++
					hash := "unrelated"
					if page == 2 {
						hash = input.Hash
					}
					return pan.OfflinePage{PageCount: 3, Tasks: []pan.OfflineTask{{Hash: hash, Status: 1, Progress: 50}}}, nil
				},
			}
			if action == "lookup" {
				remote, err := service.findRemoteTask(t.Context(), service.drive.snapshot(), input.Hash)
				if err != nil || remote.Hash != input.Hash {
					t.Fatalf("lookup=%+v err=%v", remote, err)
				}
			} else {
				if err := service.Sync(t.Context()); err != nil {
					t.Fatal(err)
				}
				if service.database.Task.GetX(t.Context(), record.ID).Progress != 50 {
					t.Fatal("sync missed the second remote page")
				}
			}
			if fetched != 2 {
				t.Fatalf("fetched %d pages, want 2", fetched)
			}
		})
	}
}
