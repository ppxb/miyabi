package scrape

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
)

func TestCorrectedExportCleanupPreservesVideosAndCanReplay(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := &Service{db: store.Client}
	record := store.Client.Movie.Create().SetCode("IPX-123").SetMetadataSnapshot(&domain.MetadataSnapshot{Code: "ABP-001"}).SaveX(t.Context())
	root := t.TempDir()
	oldDir, newDir := EmbyMovieDir(root, "ABP-001"), EmbyMovieDir(root, "IPX-123")
	for _, dir := range []string{oldDir, newDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"ABP-001.strm", "ABP-001.nfo", "poster.jpg", "fanart.jpg", "original.mp4", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(oldDir, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(newDir, "IPX-123.strm"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		path, err := service.removePreviousExport(t.Context(), root, record, record.Code)
		if err != nil || path != oldDir {
			t.Fatalf("cleanup = %q, %v", path, err)
		}
	}
	for _, name := range []string{"ABP-001.strm", "ABP-001.nfo", "poster.jpg", "fanart.jpg"} {
		if _, err := os.Stat(filepath.Join(oldDir, name)); !os.IsNotExist(err) {
			t.Fatalf("old export remains: %s", name)
		}
	}
	for _, path := range []string{filepath.Join(oldDir, "original.mp4"), filepath.Join(oldDir, "notes.txt"), filepath.Join(newDir, "IPX-123.strm")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("deleted retained file: %s", path)
		}
	}
	store.Client.Movie.Create().SetCode("ABP-001").ExecX(t.Context())
	if err := os.WriteFile(filepath.Join(oldDir, "ABP-001.strm"), []byte("another movie"), 0644); err != nil {
		t.Fatal(err)
	}
	if path, err := service.removePreviousExport(t.Context(), root, record, record.Code); err != nil || path != "" {
		t.Fatal("deleted another movie's export")
	}
	if _, err := os.Stat(filepath.Join(oldDir, "ABP-001.strm")); err != nil {
		t.Fatal(err)
	}
}
