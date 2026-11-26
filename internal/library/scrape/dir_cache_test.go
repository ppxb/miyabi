package scrape

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
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

func TestDirectoriesChecksAllPartsAndRefreshesStaleCache(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	source := domain.LibrarySource{
		AccountID: "acc-1",
		Directory: domain.LibraryDirectory{ID: "10"},
	}
	film := store.Client.Movie.Create().SetCode("ABP-001").SaveX(ctx)
	store.Client.File.Create().
		SetFileID("vid-1").SetName("ABP-001-CD1.mp4").SetSize(domain.MinVideoSize).
		SetParentID("20").SetAccountID("acc-1").SetRootID("10").SetMovie(film).ExecX(ctx)
	store.Client.File.Create().
		SetFileID("vid-2").SetName("ABP-001-CD2.mp4").SetSize(domain.MinVideoSize).
		SetParentID("20").SetAccountID("acc-1").SetRootID("10").SetMovie(film).ExecX(ctx)

	payload := MetadataPayload{Source: source, MovieID: film.ID, Code: film.Code}

	t.Run("stale cache missing a newly added part refreshes once without calling Info", func(t *testing.T) {
		service := &Service{
			db: store.Client,
			dirCache: map[string]dirCacheEntry{
				"acc-1:20": {
					files:     []pan.File{{ID: "vid-1", Name: "ABP-001-CD1.mp4", Size: domain.MinVideoSize}},
					expiresAt: time.Now().Add(defaultDirCacheTTL),
				},
			},
		}
		sess := &mockSession{
			source: source,
			listFunc: func(context.Context, string, int) (pan.FilePage, error) {
				return pan.FilePage{
					Total: 2,
					Files: []pan.File{
						{ID: "vid-1", Name: "ABP-001-CD1.mp4", Size: domain.MinVideoSize},
						{ID: "vid-2", Name: "ABP-001-CD2.mp4", Size: domain.MinVideoSize},
					},
					Path: []pan.Directory{{ID: "10"}, {ID: "20"}},
				}, nil
			},
		}
		dirs, err := service.Directories(ctx, sess, payload)
		if err != nil {
			t.Fatalf("expected refreshed listing to succeed, got %v", err)
		}
		if len(dirs) != 1 || len(dirs[0].Files) != 2 || sess.listCalls.Load() != 1 || sess.infoCalls.Load() != 0 {
			t.Fatalf("unexpected Directories result: dirs=%+v list=%d info=%d", dirs, sess.listCalls.Load(), sess.infoCalls.Load())
		}
	})

	t.Run("missing part in fresh listing fails with KindNotFound without calling Info", func(t *testing.T) {
		service := &Service{
			db:       store.Client,
			dirCache: make(map[string]dirCacheEntry),
		}
		sess := &mockSession{
			source: source,
			listFunc: func(context.Context, string, int) (pan.FilePage, error) {
				return pan.FilePage{
					Total: 1,
					Files: []pan.File{{ID: "vid-1", Name: "ABP-001-CD1.mp4", Size: domain.MinVideoSize}},
					Path:  []pan.Directory{{ID: "10"}, {ID: "20"}},
				}, nil
			},
		}
		_, err := service.Directories(ctx, sess, payload)
		if !domain.IsKind(err, domain.KindNotFound) {
			t.Fatalf("expected KindNotFound for missing part, got %v", err)
		}
		if sess.listCalls.Load() != 1 || sess.infoCalls.Load() != 0 {
			t.Fatalf("unexpected remote calls: list=%d info=%d", sess.listCalls.Load(), sess.infoCalls.Load())
		}
	})

	t.Run("publish rechecks a complete cached listing", func(t *testing.T) {
		images, err := mediaimage.NewCache(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		var cover bytes.Buffer
		if err := png.Encode(&cover, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
			t.Fatal(err)
		}
		artwork, err := images.Restore(cover.Bytes(), cover.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		parts := []pan.File{
			{ID: "vid-1", Name: "ABP-001-CD1.mp4", Size: domain.MinVideoSize},
			{ID: "vid-2", Name: "ABP-001-CD2.mp4", Size: domain.MinVideoSize},
		}
		for _, test := range []struct {
			name   string
			files  []pan.File
			rootID string
			want   domain.Kind
		}{
			{"one part removed", parts[:1], "10", domain.KindNotFound},
			{"directory moved outside source", parts, "99", domain.KindConflict},
		} {
			t.Run(test.name, func(t *testing.T) {
				service := &Service{db: store.Client, images: images}
				page := pan.FilePage{Total: len(parts), Files: parts, Path: []pan.Directory{{ID: "10"}, {ID: "20"}}}
				sess := &mockSession{source: source, listFunc: func(context.Context, string, int) (pan.FilePage, error) {
					return page, nil
				}}
				if _, err := service.Directories(ctx, sess, payload); err != nil {
					t.Fatal(err)
				}
				page = pan.FilePage{Total: len(test.files), Files: test.files, Path: []pan.Directory{{ID: test.rootID}, {ID: "20"}}}
				if _, err := service.Directories(ctx, sess, payload); err != nil || sess.listCalls.Load() != 1 {
					t.Fatalf("metadata should reuse the complete cache: err=%v list=%d", err, sess.listCalls.Load())
				}
				before := sess.listCalls.Load()
				err := service.publishMovie(ctx, sess, tasks.Job{}, Payload{MetadataPayload: payload, Artwork: &artwork})
				if calls := sess.listCalls.Load() - before; !domain.IsKind(err, test.want) || calls != 1 || sess.infoCalls.Load() != 0 {
					t.Fatalf("publish: want kind %v, one List and no Info; got err=%v list=%d info=%d", test.want, err, calls, sess.infoCalls.Load())
				}
			})
		}
	})
}
