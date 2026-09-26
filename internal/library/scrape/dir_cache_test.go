package scrape

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/pan"
)

type mockSession struct {
	source    domain.LibrarySource
	listCalls atomic.Int32
	infoCalls atomic.Int32
	listFunc  func(ctx context.Context, dirID string, offset int) (pan.FilePage, error)
	infoFunc  func(ctx context.Context, fileID string) (pan.FileInfo, error)
}

func (m *mockSession) Source() domain.LibrarySource { return m.source }
func (m *mockSession) Version() uint64              { return 1 }
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
func (m *mockSession) Read(ctx context.Context, pickCode string, limit int64) ([]byte, error) {
	return nil, nil
}
func (m *mockSession) Upload(ctx context.Context, dirID, name string, body []byte) error {
	return nil
}
func (m *mockSession) Commit(ctx context.Context, fn func(tx *ent.Tx) error) error {
	return nil
}
func (m *mockSession) CommitAccount(ctx context.Context, fn func(tx *ent.Tx) error) error {
	return nil
}
func (m *mockSession) PlayURL(ctx context.Context, pickCode string) ([]pan.PlaySource, error) {
	return nil, nil
}
func (m *mockSession) DownloadURL(ctx context.Context, pickCode, userAgent string) (string, error) {
	return "", nil
}
func (m *mockSession) AddOffline(ctx context.Context, magnet string) (string, error) {
	return "", nil
}
func (m *mockSession) RemoveOffline(ctx context.Context, hash string) error {
	return nil
}
func (m *mockSession) OfflineTasks(ctx context.Context, page int) (pan.OfflinePage, error) {
	return pan.OfflinePage{}, nil
}

func TestDirectoryEntries_CacheAndInvalidation(t *testing.T) {
	service := &Service{
		dirCache: make(map[string]dirCacheEntry),
		dirTTL:   time.Minute,
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

	// Second read within TTL: should hit cache
	files2, err := service.directoryEntries(ctx, sess, "dir-100")
	if err != nil {
		t.Fatalf("second directoryEntries failed: %v", err)
	}
	if len(files2) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files2))
	}
	if sess.listCalls.Load() != 1 {
		t.Fatalf("expected still 1 list call (cache hit), got %d", sess.listCalls.Load())
	}

	// Invalidate cache
	service.InvalidateDirCache("acc-1", "dir-100")

	// Third read after invalidation: should query session again
	files3, err := service.directoryEntries(ctx, sess, "dir-100")
	if err != nil {
		t.Fatalf("third directoryEntries failed: %v", err)
	}
	if len(files3) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files3))
	}
	if sess.listCalls.Load() != 2 {
		t.Fatalf("expected 2 list calls after invalidation, got %d", sess.listCalls.Load())
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
			return pan.FileInfo{}, errors.New("file not found on 115")
		},
	}
	errDeleted := service.verifyVideoPositions(t.Context(), sessDeleted, dir)
	if !domain.IsKind(errDeleted, domain.KindNotFound) {
		t.Fatalf("expected KindNotFound for deleted video, got %v", errDeleted)
	}
}
