package scrape

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/pan"
)

type mockSession struct {
	drive.Session // These cache tests exercise only Source, List and Info.
	source        domain.LibrarySource
	listCalls     atomic.Int32
	infoCalls     atomic.Int32
	listFunc      func(ctx context.Context, dirID string, offset int) (pan.FilePage, error)
	infoFunc      func(ctx context.Context, fileID string) (pan.FileInfo, error)
}

func (m *mockSession) Source() domain.LibrarySource { return m.source }
func (m *mockSession) List(ctx context.Context, dirID string, offset int) (pan.FilePage, error) {
	m.listCalls.Add(1)
	if m.listFunc != nil {
		return m.listFunc(ctx, dirID, offset)
	}
	return pan.FilePage{Total: 0, Files: nil}, nil
}
func (m *mockSession) Info(ctx context.Context, fileID string) (pan.FileInfo, error) {
	m.infoCalls.Add(1)
	if m.infoFunc != nil {
		return m.infoFunc(ctx, fileID)
	}
	return pan.FileInfo{File: pan.File{ID: fileID}}, nil
}
func TestDirectoryEntries_CacheAndExpiration(t *testing.T) {
	service := &Service{
		dirCache: make(map[string]dirCacheEntry),
	}

	source := domain.LibrarySource{
		AccountID: "acc-1",
		Directory: domain.LibraryDirectory{ID: "root-dir"},
	}
	sess := &mockSession{
		source: source,
		listFunc: func(ctx context.Context, dirID string, offset int) (pan.FilePage, error) {
			return pan.FilePage{
				Total: 2,
				Files: []pan.File{
					{ID: "v1", Name: "TEST-001.mp4", Size: domain.MinVideoSize},
					{ID: "n1", Name: "TEST-001.nfo", Size: 100},
				},
				Path: []pan.Directory{{ID: "root-dir"}},
			}, nil
		},
	}

	ctx := t.Context()

	// First read: should query session
	files1, err := service.directoryEntries(ctx, sess, "dir-100")
	if err != nil {
		t.Fatalf("first directoryEntries failed: %v", err)
	}
	if len(files1) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files1))
	}
	if sess.listCalls.Load() != 1 {
		t.Fatalf("expected 1 list call, got %d", sess.listCalls.Load())
	}
	files1[0].Name = "changed after cache miss"

	// Second read within TTL: should hit cache
	files2, err := service.directoryEntries(ctx, sess, "dir-100")
	if err != nil {
		t.Fatalf("second directoryEntries failed: %v", err)
	}
	if len(files2) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files2))
	}
	if files2[0].Name != "TEST-001.mp4" {
		t.Fatal("caller modified the cached listing after a miss")
	}
	files2[0].Name = "changed after cache hit"
	if files, err := service.directoryEntries(ctx, sess, "dir-100"); err != nil || files[0].Name != "TEST-001.mp4" {
		t.Fatalf("caller modified the cached listing after a hit: %v, %v", files, err)
	}
	if sess.listCalls.Load() != 1 {
		t.Fatalf("expected still 1 list call (cache hit), got %d", sess.listCalls.Load())
	}

	// Expire the cached listing so the next read queries the source again.
	entry := service.dirCache["acc-1:dir-100"]
	entry.expiresAt = time.Now().Add(-time.Second)
	service.dirCache["acc-1:dir-100"] = entry

	// Third read after expiration: should query session again
	files3, err := service.directoryEntries(ctx, sess, "dir-100")
	if err != nil {
		t.Fatalf("third directoryEntries failed: %v", err)
	}
	if len(files3) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files3))
	}
	if sess.listCalls.Load() != 2 {
		t.Fatalf("expected 2 list calls after expiration, got %d", sess.listCalls.Load())
	}
}

func TestDirectoryEntries_ReclaimsExpiredListings(t *testing.T) {
	for _, test := range []struct {
		name  string
		dirID string
		err   error
	}{
		{name: "cache hit", dirID: "live"},
		{name: "cache miss", dirID: "new"},
		{name: "failed request", dirID: "new", err: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &Service{dirCache: map[string]dirCacheEntry{
				"acc:expired": {files: []pan.File{{ID: "old"}}, expiresAt: time.Now().Add(-time.Second)},
				"acc:live":    {files: []pan.File{{ID: "current"}}, expiresAt: time.Now().Add(defaultDirCacheTTL)},
			}}
			sess := &mockSession{
				source: domain.LibrarySource{AccountID: "acc", Directory: domain.LibraryDirectory{ID: "root"}},
				listFunc: func(context.Context, string, int) (pan.FilePage, error) {
					return pan.FilePage{Total: 1, Files: []pan.File{{ID: "new"}}, Path: []pan.Directory{{ID: "root"}}}, test.err
				},
			}
			_, err := service.directoryEntries(t.Context(), sess, test.dirID)
			if !errors.Is(err, test.err) {
				t.Fatalf("expected error %v, got %v", test.err, err)
			}
			if _, ok := service.dirCache["acc:expired"]; ok {
				t.Fatal("expired listing retained after accessing another directory")
			}
			if _, ok := service.dirCache["acc:live"]; !ok {
				t.Fatal("unexpired listing was removed")
			}
			if test.err != nil {
				if _, ok := service.dirCache["acc:new"]; ok {
					t.Fatal("failed request was cached")
				}
			}
		})
	}
}

func TestDirectoryEntries_Capacity(t *testing.T) {
	for _, test := range []struct {
		name        string
		directories int
		filesPerDir int
		retained    int
	}{
		{name: "empty directories", directories: maxDirCacheEntries + 1, retained: maxDirCacheEntries},
		{name: "total files", directories: 3, filesPerDir: maxDirCacheFiles / 2, retained: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &Service{dirCache: make(map[string]dirCacheEntry)}
			sess := &mockSession{
				source: domain.LibrarySource{AccountID: "acc", Directory: domain.LibraryDirectory{ID: "root"}},
				listFunc: func(context.Context, string, int) (pan.FilePage, error) {
					return pan.FilePage{Total: test.filesPerDir, Files: make([]pan.File, test.filesPerDir), Path: []pan.Directory{{ID: "root"}}}, nil
				},
			}
			// Distinct future deadlines make the eviction order deterministic without sleeping.
			expiry := time.Now().Add(defaultDirCacheTTL / 2)
			for i := range test.directories {
				dirID := fmt.Sprintf("dir-%d", i)
				files, err := service.directoryEntries(t.Context(), sess, dirID)
				if err != nil || len(files) != test.filesPerDir {
					t.Fatalf("directory %s: got %d files, error %v", dirID, len(files), err)
				}
				entry, ok := service.dirCache["acc:"+dirID]
				if !ok {
					t.Fatalf("new listing %s was not cached", dirID)
				}
				entry.expiresAt = expiry.Add(time.Duration(i) * time.Millisecond)
				service.dirCache["acc:"+dirID] = entry
				fileCount := 0
				for _, cached := range service.dirCache {
					fileCount += len(cached.files)
				}
				if len(service.dirCache) > maxDirCacheEntries || fileCount > maxDirCacheFiles {
					t.Fatalf("cache exceeds budget: %d directories, %d files", len(service.dirCache), fileCount)
				}
			}
			if len(service.dirCache) != test.retained {
				t.Fatalf("expected %d retained listings, got %d", test.retained, len(service.dirCache))
			}
			if _, ok := service.dirCache["acc:dir-0"]; ok {
				t.Fatal("oldest listing was not evicted")
			}
			if _, err := service.directoryEntries(t.Context(), sess, "dir-0"); err != nil {
				t.Fatalf("reload evicted directory: %v", err)
			}
			if got := sess.listCalls.Load(); got != int32(test.directories+1) {
				t.Fatalf("expected evicted directory to be fetched again, got %d requests", got)
			}
		})
	}
}

func TestDirectoryEntries_OversizedDirectory(t *testing.T) {
	service := &Service{dirCache: map[string]dirCacheEntry{
		"acc:small": {files: []pan.File{{ID: "small"}}, expiresAt: time.Now().Add(defaultDirCacheTTL)},
	}}
	sess := &mockSession{
		source: domain.LibrarySource{AccountID: "acc", Directory: domain.LibraryDirectory{ID: "root"}},
		listFunc: func(context.Context, string, int) (pan.FilePage, error) {
			return pan.FilePage{Total: maxDirCacheFiles + 1, Files: make([]pan.File, maxDirCacheFiles+1), Path: []pan.Directory{{ID: "root"}}}, nil
		},
	}
	for range 2 {
		files, err := service.directoryEntries(t.Context(), sess, "large")
		if err != nil || len(files) != maxDirCacheFiles+1 {
			t.Fatalf("large directory was truncated: %d files, error %v", len(files), err)
		}
	}
	if sess.listCalls.Load() != 2 || len(service.dirCache) != 1 {
		t.Fatal("oversized listing should not be cached or displace other listings")
	}
	if _, ok := service.dirCache["acc:small"]; !ok {
		t.Fatal("oversized listing displaced the existing listing")
	}
}

func TestDirectoryEntries_AccountIsolation(t *testing.T) {
	service := &Service{dirCache: make(map[string]dirCacheEntry)}
	for _, account := range []string{"acc-1", "acc-2"} {
		sess := &mockSession{
			source: domain.LibrarySource{AccountID: account, Directory: domain.LibraryDirectory{ID: "root"}},
			listFunc: func(context.Context, string, int) (pan.FilePage, error) {
				return pan.FilePage{Total: 1, Files: []pan.File{{ID: account}}, Path: []pan.Directory{{ID: "root"}}}, nil
			},
		}
		for range 2 {
			files, err := service.directoryEntries(t.Context(), sess, "shared-dir-id")
			if err != nil || len(files) != 1 || files[0].ID != account {
				t.Fatalf("account %s got another account's listing: %v, %v", account, files, err)
			}
		}
		if sess.listCalls.Load() != 1 {
			t.Fatalf("account %s did not reuse its listing", account)
		}
	}
}

func TestVerifyVideoPositions(t *testing.T) {
	service := &Service{}
	source := domain.LibrarySource{
		AccountID: "acc-1",
		Directory: domain.LibraryDirectory{ID: "10"},
	}

	dir := MovieDirectory{
		ID:       "20",
		VideoIDs: map[string]bool{"vid-1": true},
	}

	// 1. Success case: video is in the directory and within source
	sessSuccess := &mockSession{
		source: source,
		infoFunc: func(ctx context.Context, fileID string) (pan.FileInfo, error) {
			return pan.FileInfo{
				File: pan.File{ID: "vid-1", ParentID: "20"},
				Path: []pan.Directory{{ID: "10"}, {ID: "20"}},
			}, nil
		},
	}
	if err := service.verifyVideoPositions(t.Context(), sessSuccess, dir); err != nil {
		t.Fatalf("expected verifyVideoPositions to succeed, got %v", err)
	}

	// 2. Moved case: video's ParentID no longer matches directory ID
	sessMoved := &mockSession{
		source: source,
		infoFunc: func(ctx context.Context, fileID string) (pan.FileInfo, error) {
			return pan.FileInfo{
				File: pan.File{ID: "vid-1", ParentID: "999"}, // moved!
				Path: []pan.Directory{{ID: "10"}, {ID: "999"}},
			}, nil
		},
	}
	errMoved := service.verifyVideoPositions(t.Context(), sessMoved, dir)
	if !domain.IsKind(errMoved, domain.KindConflict) {
		t.Fatalf("expected KindConflict for moved video, got %v", errMoved)
	}

	// 3. Deleted / Not found case: info returns error
	sessDeleted := &mockSession{
		source: source,
		infoFunc: func(ctx context.Context, fileID string) (pan.FileInfo, error) {
			return pan.FileInfo{}, pan.ErrNotFound
		},
	}
	errDeleted := service.verifyVideoPositions(t.Context(), sessDeleted, dir)
	if !domain.IsKind(errDeleted, domain.KindNotFound) {
		t.Fatalf("expected KindNotFound for deleted video, got %v", errDeleted)
	}
	sessDeleted.infoFunc = func(context.Context, string) (pan.FileInfo, error) { return pan.FileInfo{}, context.DeadlineExceeded }
	if _, retry := domain.RetryDelay(service.verifyVideoPositions(t.Context(), sessDeleted, dir)); !retry {
		t.Fatal("network timeout was classified as a deleted video")
	}
}
