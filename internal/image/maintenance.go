package image

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type CacheStats struct {
	SizeBytes        int64 `json:"size_bytes"`
	EntryCount       int   `json:"entry_count"`
	UnusedSizeBytes  int64 `json:"unused_size_bytes"`
	UnusedEntryCount int   `json:"unused_entry_count"`
}

type cachedFile struct {
	name string
	url  string
	size int64
}

func (cache *Cache) Stats(ctx context.Context, retained map[string]bool) (CacheStats, error) {
	files, err := cache.files(ctx)
	if err != nil {
		return CacheStats{}, err
	}
	var stats CacheStats
	for _, file := range files {
		stats.SizeBytes += file.size
		stats.EntryCount++
		if !retained[file.url] {
			stats.UnusedSizeBytes += file.size
			stats.UnusedEntryCount++
		}
	}
	return stats, nil
}

// Prune removes only cache-owned JPEG files. The caller must serialize the
// reference snapshot and pruning with artwork generation and publication.
func (cache *Cache) Prune(ctx context.Context, retained map[string]bool) error {
	files, err := cache.files(ctx)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if retained[file.url] {
			continue
		}
		if err := os.Remove(filepath.Join(cache.directory, file.name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove unused artwork: %w", err)
		}
	}
	return nil
}

func (cache *Cache) files(ctx context.Context) ([]cachedFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(cache.directory)
	if err != nil {
		return nil, fmt.Errorf("read image cache: %w", err)
	}
	files := make([]cachedFile, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := entry.Name()
		key, found := strings.CutSuffix(name, ".jpg")
		if !found || len(key) != 64 || entry.Type()&os.ModeType != 0 {
			continue
		}
		if _, err := hex.DecodeString(key); err != nil {
			continue
		}
		info, err := entry.Info()
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read cached image size: %w", err)
		}
		if info.Mode().IsRegular() {
			files = append(files, cachedFile{name: name, url: URLPrefix + key, size: info.Size()})
		}
	}
	return files, nil
}
