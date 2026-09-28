package scrape

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/export"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
)

// Re-export constants, types, and functions from internal/export.
const STRMPlayPath = export.STRMPlayPath

type (
	ExportConfig  = export.Config
	ExportManager = export.Manager
	MediaNotifier = export.MediaNotifier
)

var (
	NewExportManager = export.NewManager
	ParseSTRMFileID  = export.ParseSTRMFileID
	EmbyMovieDir     = export.EmbyMovieDir
	STRMContent      = export.STRMContent
)

// RewriteSTRM forwards to export.RewriteSTRM with a background context.
func RewriteSTRM(embyDir, publicURL, strmToken string) (int, error) {
	return export.RewriteSTRM(context.Background(), embyDir, publicURL, strmToken)
}

// NaturalCompare compares two strings using natural numeric order (e.g. "cd2" < "cd10").
func NaturalCompare(a, b string) int {
	aLower, bLower := strings.ToLower(a), strings.ToLower(b)
	i, j := 0, 0
	for i < len(aLower) && j < len(bLower) {
		if isDigit(aLower[i]) && isDigit(bLower[j]) {
			startA, startB := i, j
			for i < len(aLower) && isDigit(aLower[i]) {
				i++
			}
			for j < len(bLower) && isDigit(bLower[j]) {
				j++
			}
			numA := strings.TrimLeft(aLower[startA:i], "0")
			numB := strings.TrimLeft(bLower[startB:j], "0")
			if len(numA) != len(numB) {
				return cmp.Compare(len(numA), len(numB))
			}
			if c := cmp.Compare(numA, numB); c != 0 {
				return c
			}
			continue
		}
		if aLower[i] != bLower[j] {
			return cmp.Compare(aLower[i], bLower[j])
		}
		i++
		j++
	}
	return cmp.Compare(len(aLower)-i, len(bLower)-j)
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// SortFiles sorts video files by Name using natural sort order.
func SortFiles(videos []pan.File) {
	slices.SortFunc(videos, func(a, b pan.File) int {
		if c := NaturalCompare(a.Name, b.Name); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
}

// ExportEmbyMedia writes .strm, .nfo, poster.jpg, and fanart.jpg files to the Emby directory structure.
func ExportEmbyMedia(embyDir, publicURL, strmToken, code string, doc nfo.Movie, videos []pan.File, poster, fanart []byte) error {
	stem := nfo.FileStem(code)
	destDir := export.EmbyMovieDir(embyDir, code)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create emby directory %s: %w", destDir, err)
	}

	// Ensure videos are sorted deterministically
	SortFiles(videos)

	// 1. Write poster and fanart FIRST
	posterName, fanartName := "poster.jpg", "fanart.jpg"
	if len(poster) > 0 {
		posterPath := filepath.Join(destDir, posterName)
		if err := os.WriteFile(posterPath, poster, 0o644); err != nil {
			return fmt.Errorf("write poster: %w", err)
		}
	}
	if len(fanart) > 0 {
		fanartPath := filepath.Join(destDir, fanartName)
		if err := os.WriteFile(fanartPath, fanart, 0o644); err != nil {
			return fmt.Errorf("write fanart: %w", err)
		}
	}

	// 2. Write NFO file SECOND
	nfoName := stem + ".nfo"
	doc.Thumbs = []nfo.Thumb{{Aspect: "poster", Path: posterName}}
	doc.Fanart = fanartName
	nfoBody, err := nfo.Encode(doc)
	if err != nil {
		return fmt.Errorf("encode nfo: %w", err)
	}
	nfoPath := filepath.Join(destDir, nfoName)
	if err := os.WriteFile(nfoPath, nfoBody, 0o644); err != nil {
		return fmt.Errorf("write nfo file: %w", err)
	}

	// 3. Write STRM files LAST so media servers (Emby) watching via inotify detect complete assets
	expectedSTRMs := make(map[string]bool)
	if len(videos) == 1 {
		strmName := stem + ".strm"
		expectedSTRMs[strmName] = true
		strmPath := filepath.Join(destDir, strmName)
		if err := os.WriteFile(strmPath, export.STRMContent(publicURL, videos[0].ID, strmToken), 0o644); err != nil {
			return fmt.Errorf("write strm file: %w", err)
		}
	} else if len(videos) > 1 {
		for i, v := range videos {
			strmName := fmt.Sprintf("%s-cd%d.strm", stem, i+1)
			expectedSTRMs[strmName] = true
			strmPath := filepath.Join(destDir, strmName)
			if err := os.WriteFile(strmPath, export.STRMContent(publicURL, v.ID, strmToken), 0o644); err != nil {
				return fmt.Errorf("write strm file: %w", err)
			}
		}
	}

	// Clean up obsolete .strm files belonging to this movie
	if entries, err := os.ReadDir(destDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.HasSuffix(name, ".strm") && strings.HasPrefix(name, stem) && !expectedSTRMs[name] {
				_ = os.Remove(filepath.Join(destDir, name))
			}
		}
	}

	return nil
}

// ExportLocalMovie exports an already-scraped ent.Movie record and cached artwork to the Emby directory if missing.
func ExportLocalMovie(embyDir, publicURL, strmToken string, record *ent.Movie, images *mediaimage.Cache, notifiers ...MediaNotifier) error {
	if record == nil || record.Code == "" {
		return nil
	}

	destDir := export.EmbyMovieDir(embyDir, record.Code)
	stem := nfo.FileStem(record.Code)
	nfoPath := filepath.Join(destDir, stem+".nfo")
	posterPath := filepath.Join(destDir, "poster.jpg")

	hasSTRM := false
	if len(record.Edges.Files) == 1 {
		_, err := os.Stat(filepath.Join(destDir, stem+".strm"))
		hasSTRM = err == nil
	} else if len(record.Edges.Files) > 1 {
		_, err := os.Stat(filepath.Join(destDir, fmt.Sprintf("%s-cd1.strm", stem)))
		hasSTRM = err == nil
	}

	_, nfoErr := os.Stat(nfoPath)
	_, posterErr := os.Stat(posterPath)
	if nfoErr == nil && posterErr == nil && hasSTRM {
		return nil
	}

	doc := MovieNFO(record)
	videos := make([]pan.File, 0, len(record.Edges.Files))
	for _, f := range record.Edges.Files {
		videos = append(videos, pan.File{ID: f.FileID, Name: f.Name, Size: f.Size, PickCode: f.PickCode})
	}
	SortFiles(videos)

	var posterBytes, fanartBytes []byte
	if images != nil {
		artwork := MovieArtwork(record)
		if artwork.Poster != "" {
			posterBytes, _ = images.ReadURL(artwork.Poster)
		}
		if artwork.Fanart != "" {
			fanartBytes, _ = images.ReadURL(artwork.Fanart)
		} else if artwork.Thumbnail != "" {
			fanartBytes, _ = images.ReadURL(artwork.Thumbnail)
		}
	}

	if err := ExportEmbyMedia(embyDir, publicURL, strmToken, record.Code, doc, videos, posterBytes, fanartBytes); err != nil {
		return err
	}
	if len(notifiers) > 0 && notifiers[0] != nil {
		notifiers[0].NotifyUpdated(destDir)
	}
	return nil
}
