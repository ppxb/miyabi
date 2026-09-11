package image

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	stdimage "image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestFromCoverConcurrentSavesAreIdempotent(t *testing.T) {
	cover := stdimage.NewRGBA(stdimage.Rect(0, 0, 48, 32))
	for y := range 32 {
		for x := range 48 {
			cover.SetRGBA(x, y, color.RGBA{R: uint8(x * 5), G: uint8(y * 7), B: 90, A: 255})
		}
	}
	var body bytes.Buffer
	if err := png.Encode(&body, cover); err != nil {
		t.Fatal(err)
	}

	for attempt := range 5 {
		t.Run(fmt.Sprint(attempt), func(t *testing.T) {
			directory := t.TempDir()
			var caches [2]*Cache
			for i := range caches {
				cache, err := NewCache(directory)
				if err != nil {
					t.Fatal(err)
				}
				caches[i] = cache
			}
			type result struct {
				artwork Artwork
				err     error
			}
			const writers = 32
			start := make(chan struct{})
			results := make(chan result, writers)
			var workers sync.WaitGroup
			for i := range writers {
				workers.Go(func() {
					<-start
					artwork, err := caches[i%len(caches)].FromCover(body.Bytes())
					results <- result{artwork: artwork, err: err}
				})
			}
			close(start)
			workers.Wait()
			close(results)

			var expected Artwork
			for result := range results {
				if result.err != nil {
					t.Errorf("concurrent save failed: %v", result.err)
					continue
				}
				if expected == (Artwork{}) {
					expected = result.artwork
				}
				if result.artwork != expected {
					t.Errorf("concurrent save returned different artwork: %+v, want %+v", result.artwork, expected)
				}
			}
			for _, url := range []string{expected.Poster, expected.Fanart, expected.Thumbnail} {
				data, err := caches[0].ReadURL(url)
				if err != nil {
					t.Fatal(err)
				}
				sum := sha256.Sum256(data)
				if url != URLPrefix+hex.EncodeToString(sum[:]) {
					t.Errorf("cached content does not match its key: %s", url)
				}
				if _, format, err := stdimage.Decode(bytes.NewReader(data)); err != nil || format != "jpeg" {
					t.Errorf("cached image is not a complete JPEG: format=%s, error=%v", format, err)
				}
			}
			entries, err := os.ReadDir(caches[0].directory)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".tmp") {
					t.Errorf("temporary image was not removed: %s", entry.Name())
				}
			}
		})
	}
}

func TestSaveRejectsDirectoryAtCacheKey(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cover := stdimage.NewRGBA(stdimage.Rect(0, 0, 2, 2))
	url, err := cache.save(cover)
	if err != nil {
		t.Fatal(err)
	}
	name, err := cache.filePath(strings.TrimPrefix(url, URLPrefix))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(name, 0o755); err != nil {
		t.Fatal(err)
	}
	if url, err := cache.save(cover); err == nil || url != "" {
		t.Fatalf("directory reported as cached image: url=%q, error=%v", url, err)
	}
}

func TestSaveReportsMissingCacheDirectory(t *testing.T) {
	cache := &Cache{directory: filepath.Join(t.TempDir(), "missing")}
	if url, err := cache.save(stdimage.NewRGBA(stdimage.Rect(0, 0, 2, 2))); !os.IsNotExist(err) || url != "" {
		t.Fatalf("missing cache directory reported as success: url=%q, error=%v", url, err)
	}
	if err := os.Mkdir(cache.directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.save(stdimage.NewRGBA(stdimage.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("failed write prevented a later retry: %v", err)
	}
}

func TestWriteCleansTemporaryFileOnPublishFailure(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(cache.directory, "missing", "image.jpg")
	if err := cache.write(destination, []byte("complete image")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing destination parent error was lost: %v", err)
	}
	entries, err := os.ReadDir(cache.directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed publication left temporary files: %v", entries)
	}
}

func TestPublishImageOnlyAcceptsMatchingCompletedFile(t *testing.T) {
	body := []byte("complete image")
	for _, test := range []struct {
		name      string
		data      []byte
		directory bool
		wantErr   bool
	}{
		{name: "matching", data: body},
		{name: "different same size", data: []byte("different data"), wantErr: true},
		{name: "truncated", data: body[:3], wantErr: true},
		{name: "missing", wantErr: true},
		{name: "directory", directory: true, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			destination := filepath.Join(dir, "image.jpg")
			if test.directory {
				if err := os.Mkdir(destination, 0o755); err != nil {
					t.Fatal(err)
				}
			} else if test.data != nil {
				if err := os.WriteFile(destination, test.data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			// A missing source forces a rename error on every platform, even when
			// the destination already contains the exact image another writer saved.
			temporary := filepath.Join(dir, "missing.tmp")
			err := publishImage(temporary, destination, body)
			if (err != nil) != test.wantErr {
				t.Fatalf("publish error=%v, want error=%t", err, test.wantErr)
			}
			if test.wantErr {
				var renameErr *os.LinkError
				if !errors.As(err, &renameErr) || renameErr.Old != temporary || renameErr.New != destination {
					t.Fatalf("original rename error was lost: %v", err)
				}
			}
		})
	}
}
