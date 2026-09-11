package image

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCachePruneOnlyRemovesUnreferencedManagedImages(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	kept, unused := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for name, body := range map[string]string{
		kept + ".jpg": "keep", unused + ".jpg": "unused",
		"notes.txt": "not owned by the cache", "image-partial.tmp": "pending",
		strings.Repeat("z", 64) + ".jpg": "not a cache key",
	} {
		if err := os.WriteFile(filepath.Join(cache.directory, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	directory := filepath.Join(cache.directory, strings.Repeat("c", 64)+".jpg")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "keep.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	retained := map[string]bool{URLPrefix + kept: true}
	stats, err := cache.Stats(t.Context(), retained)
	if err != nil || stats != (CacheStats{SizeBytes: 10, EntryCount: 2, UnusedSizeBytes: 6, UnusedEntryCount: 1}) {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
	for range 2 {
		if err := cache.Prune(t.Context(), retained); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(cache.directory, unused+".jpg")); !os.IsNotExist(err) {
		t.Fatalf("unused cache file still exists: %v", err)
	}
	for _, name := range []string{kept + ".jpg", "notes.txt", "image-partial.tmp", strings.Repeat("z", 64) + ".jpg", filepath.Join(filepath.Base(directory), "keep.txt")} {
		if _, err := os.Stat(filepath.Join(cache.directory, name)); err != nil {
			t.Fatalf("prune removed protected or unrelated file %q: %v", name, err)
		}
	}
	stats, err = cache.Stats(t.Context(), retained)
	if err != nil || stats != (CacheStats{SizeBytes: 4, EntryCount: 1}) {
		t.Fatalf("post-cleanup stats=%+v err=%v", stats, err)
	}
}

func TestCacheMaintenanceHonorsCancellationBeforeRemovingFiles(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(cache.directory, strings.Repeat("a", 64)+".jpg")
	if err := os.WriteFile(name, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := cache.Prune(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("prune error = %v", err)
	}
	if _, err := cache.Stats(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("stats error = %v", err)
	}
	if _, err := os.Stat(name); err != nil {
		t.Fatal(err)
	}
}

func TestCachePruneDoesNotFollowSymbolicLinks(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "outside.jpg")
	if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(cache.directory, strings.Repeat("a", 64)+".jpg")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if err := cache.Prune(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("cache cleanup changed the symbolic link: %v", err)
	}
	if body, err := os.ReadFile(target); err != nil || string(body) != "outside" {
		t.Fatalf("cache cleanup changed a file outside the cache: %q %v", body, err)
	}
}
