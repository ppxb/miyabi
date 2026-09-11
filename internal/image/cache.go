package image

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	stdimage "image"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"golang.org/x/sync/singleflight"
)

const URLPrefix = "/api/library/artwork/"

// Cache instances sharing a destination also share its in-flight write.
var imageWrites singleflight.Group

type Cache struct{ directory string }

type Artwork struct {
	Poster    string `json:"poster"`
	Fanart    string `json:"fanart"`
	Thumbnail string `json:"thumbnail"`
}

func NewCache(dataDir string) (*Cache, error) {
	directory := filepath.Join(dataDir, "images")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("create image cache: %w", err)
	}
	return &Cache{directory: directory}, nil
}

func (cache *Cache) FromCover(body []byte) (Artwork, error) {
	cover, err := imaging.Decode(bytes.NewReader(body), imaging.AutoOrientation(true))
	if err != nil {
		return Artwork{}, fmt.Errorf("decode cover image: %w", err)
	}
	size := cover.Bounds().Size()
	poster := imaging.CropAnchor(cover, min(size.X, size.Y*2/3), size.Y, imaging.Right)
	return cache.saveArtwork(poster, cover)
}

func (cache *Cache) Restore(poster, fanart []byte) (Artwork, error) {
	posterImage, err := imaging.Decode(bytes.NewReader(poster), imaging.AutoOrientation(true))
	if err != nil {
		return Artwork{}, fmt.Errorf("decode NFO poster: %w", err)
	}
	fanartImage, err := imaging.Decode(bytes.NewReader(fanart), imaging.AutoOrientation(true))
	if err != nil {
		return Artwork{}, fmt.Errorf("decode NFO fanart: %w", err)
	}
	return cache.saveArtwork(posterImage, fanartImage)
}

func (cache *Cache) saveArtwork(poster, fanart stdimage.Image) (Artwork, error) {
	var artwork Artwork
	var err error
	artwork.Poster, err = cache.save(imaging.Fit(poster, 800, 1200, imaging.Lanczos))
	if err != nil {
		return Artwork{}, err
	}
	artwork.Fanart, err = cache.save(fanart)
	if err != nil {
		return Artwork{}, err
	}
	artwork.Thumbnail, err = cache.save(imaging.Resize(fanart, 480, 0, imaging.Lanczos))
	return artwork, err
}

func (cache *Cache) save(source stdimage.Image) (string, error) {
	var buffer bytes.Buffer
	if err := imaging.Encode(&buffer, source, imaging.JPEG, imaging.JPEGQuality(90)); err != nil {
		return "", fmt.Errorf("encode cached image: %w", err)
	}
	sum := sha256.Sum256(buffer.Bytes())
	key := hex.EncodeToString(sum[:])
	destination, err := filepath.Abs(filepath.Join(cache.directory, key+".jpg"))
	if err != nil {
		return "", err
	}
	_, err, _ = imageWrites.Do(destination, func() (any, error) {
		return nil, cache.write(destination, buffer.Bytes())
	})
	if err != nil {
		return "", err
	}
	return URLPrefix + key, nil
}

func (cache *Cache) write(destination string, body []byte) error {
	if info, err := os.Stat(destination); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("cached image is not a regular file")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	temporary, err := os.CreateTemp(cache.directory, "image-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	_, writeErr := temporary.Write(body)
	closeErr := temporary.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := publishImage(temporary.Name(), destination, body); err != nil {
		return fmt.Errorf("store image cache: %w", err)
	}
	return nil
}

func publishImage(temporary, destination string, body []byte) error {
	err := os.Rename(temporary, destination)
	if err == nil {
		return nil
	}
	// Concurrent writers can fail to replace the same file on Windows. Accept
	// another writer's result only when it contains the complete expected image.
	if info, statErr := os.Stat(destination); statErr == nil && info.Mode().IsRegular() && info.Size() == int64(len(body)) {
		if saved, readErr := os.ReadFile(destination); readErr == nil && bytes.Equal(saved, body) {
			return nil
		}
	}
	return err
}

func (cache *Cache) Read(key string) ([]byte, error) {
	name, err := cache.filePath(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(name)
}

func (cache *Cache) filePath(key string) (string, error) {
	if len(key) != 64 {
		return "", fmt.Errorf("invalid artwork key")
	}
	if _, err := hex.DecodeString(key); err != nil {
		return "", err
	}
	return filepath.Join(cache.directory, key+".jpg"), nil
}

func (cache *Cache) ReadURL(url string) ([]byte, error) {
	if !strings.HasPrefix(url, URLPrefix) {
		return nil, fmt.Errorf("image is not in the local cache")
	}
	return cache.Read(strings.TrimPrefix(url, URLPrefix))
}

func (cache *Cache) Exists(artwork Artwork) (bool, error) {
	for _, url := range []string{artwork.Poster, artwork.Fanart, artwork.Thumbnail} {
		if url == "" {
			return false, nil
		}
		if !strings.HasPrefix(url, URLPrefix) {
			return false, fmt.Errorf("image is not in the local cache")
		}
		name, err := cache.filePath(strings.TrimPrefix(url, URLPrefix))
		if err != nil {
			return false, err
		}
		if _, err := os.Stat(name); os.IsNotExist(err) {
			return false, nil
		} else if err != nil {
			return false, err
		}
	}
	return true, nil
}
