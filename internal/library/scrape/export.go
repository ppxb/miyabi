package scrape

import (
	"bytes"
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
	_, err := exportEmbyMedia(embyDir, publicURL, strmToken, code, doc, videos, poster, fanart)
	return err
}

// Compare each output independently so repairing one missing sidecar does not
// touch complete files or trigger unnecessary media-server file events.
func exportEmbyMedia(embyDir, publicURL, strmToken, code string, doc nfo.Movie, videos []pan.File, poster, fanart []byte) (bool, error) {
	stem := nfo.FileStem(code)
	destDir := export.EmbyMovieDir(embyDir, code)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return false, fmt.Errorf("create emby directory %s: %w", destDir, err)
	}
	changed := false
	write := func(name string, body []byte) error {
		path := filepath.Join(destDir, name)
		before, err := os.ReadFile(path)
		if err == nil && bytes.Equal(before, body) {
			return nil
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			return err
		}
		changed = true
		return nil
	}

	// Ensure videos are sorted deterministically
	SortFiles(videos)

	// 1. Write poster and fanart FIRST
	posterName, fanartName := "poster.jpg", "fanart.jpg"
	if len(poster) > 0 {
		if err := write(posterName, poster); err != nil {
			return changed, fmt.Errorf("write poster: %w", err)
		}
	}
	if len(fanart) > 0 {
		if err := write(fanartName, fanart); err != nil {
			return changed, fmt.Errorf("write fanart: %w", err)
		}
	}

	// 2. Write NFO file SECOND
	nfoName := stem + ".nfo"
	doc.Thumbs = []nfo.Thumb{{Aspect: "poster", Path: posterName}}
	doc.Fanart = fanartName
	nfoBody, err := nfo.Encode(doc)
	if err != nil {
		return changed, fmt.Errorf("encode nfo: %w", err)
	}
	if err := write(nfoName, nfoBody); err != nil {
		return changed, fmt.Errorf("write nfo file: %w", err)
	}

	// 3. Write STRM files LAST so media servers (Emby) watching via inotify detect complete assets
	expectedSTRMs := make(map[string]bool)
	if len(videos) == 1 {
		strmName := stem + ".strm"
		expectedSTRMs[strmName] = true
		if err := write(strmName, export.STRMContent(publicURL, videos[0].ID, strmToken)); err != nil {
			return changed, fmt.Errorf("write strm file: %w", err)
		}
	} else if len(videos) > 1 {
		for i, v := range videos {
			strmName := fmt.Sprintf("%s-cd%d.strm", stem, i+1)
			expectedSTRMs[strmName] = true
			if err := write(strmName, export.STRMContent(publicURL, v.ID, strmToken)); err != nil {
				return changed, fmt.Errorf("write strm file: %w", err)
			}
		}
	}

	// Clean up obsolete .strm files belonging to this movie
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return changed, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".strm") && strings.HasPrefix(name, stem) && !expectedSTRMs[name] {
			if err := os.Remove(filepath.Join(destDir, name)); err != nil {
				return changed, err
			}
			changed = true
		}
	}

	return changed, nil
}

// ExportLocalMovie repairs missing or changed outputs using saved metadata and artwork.
func ExportLocalMovie(embyDir, publicURL, strmToken string, record *ent.Movie, images *mediaimage.Cache) (bool, error) {
	if record.Code == "" {
		return false, nil
	}

	doc, err := MovieNFO(record)
	if err != nil {
		return false, err
	}
	videos := make([]pan.File, 0, len(record.Edges.Files))
	for _, f := range record.Edges.Files {
		videos = append(videos, pan.File{ID: f.FileID, Name: f.Name, Size: f.Size, PickCode: f.PickCode})
	}
	var posterBytes, fanartBytes []byte
	artwork := MovieArtwork(record)
	if artwork.Poster != "" {
		posterBytes, err = images.ReadURL(artwork.Poster)
		if err != nil {
			return false, err
		}
	}
	if artwork.Fanart != "" {
		fanartBytes, err = images.ReadURL(artwork.Fanart)
	} else if artwork.Thumbnail != "" {
		fanartBytes, err = images.ReadURL(artwork.Thumbnail)
	}
	if err != nil {
		return false, err
	}
	return exportEmbyMedia(embyDir, publicURL, strmToken, record.Code, doc, videos, posterBytes, fanartBytes)
}
