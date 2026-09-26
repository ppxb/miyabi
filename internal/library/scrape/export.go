package scrape

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/ent"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/netx"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
)

const defaultEmbyDir = "./data/emby"

// STRMPlayPath is the API route prefix for media streaming playback in .strm files.
const STRMPlayPath = "/api/strm/play/"

var strmPlayRegex = regexp.MustCompile(`/api/strm/play/([a-zA-Z0-9_\-]+)`)

// ParseSTRMFileID extracts the 115 file ID from a .strm file's content or URL.
func ParseSTRMFileID(content string) string {
	match := strmPlayRegex.FindStringSubmatch(content)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func defaultPublicURL() string {
	return fmt.Sprintf("http://%s:8080", netx.OutboundIP())
}

// EmbyMovieDir is the directory holding a movie's exported Emby files,
// bucketed by catalogue prefix: <embyDir>/<prefix>/<code>.
func EmbyMovieDir(embyDir, code string) string {
	if embyDir == "" {
		embyDir = defaultEmbyDir
	}
	return filepath.Join(embyDir, codeid.Prefix(code), code)
}

// STRMContent is the body of a .strm file: the relay URL that resolves the
// 115 video to a fresh stream whenever Emby plays it.
func STRMContent(publicURL, fileID, strmToken string) []byte {
	if publicURL == "" {
		publicURL = defaultPublicURL()
	}
	publicURL = strings.TrimRight(strings.TrimSpace(publicURL), "/")
	content := publicURL + STRMPlayPath + fileID
	if strmToken != "" {
		content += "?token=" + url.QueryEscape(strmToken)
	}
	return []byte(content + "\n")
}

// RewriteSTRM walks the Emby export directory and updates all existing .strm files
// to use the active publicURL and strmToken.
// It returns the count of rewritten files.
func RewriteSTRM(embyDir, publicURL, strmToken string) (int, error) {
	if embyDir == "" {
		embyDir = defaultEmbyDir
	}
	if publicURL == "" {
		publicURL = defaultPublicURL()
	}
	publicURL = strings.TrimRight(strings.TrimSpace(publicURL), "/")

	rewritten := 0
	err := filepath.WalkDir(embyDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".strm") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		fileID := ParseSTRMFileID(string(data))
		if fileID == "" {
			return nil
		}
		newContent := STRMContent(publicURL, fileID, strmToken)
		if !bytes.Equal(bytes.TrimSpace(data), bytes.TrimSpace(newContent)) {
			if err := os.WriteFile(path, newContent, 0o644); err == nil {
				rewritten++
			}
		}
		return nil
	})
	return rewritten, err
}

// ExportEmbyMedia writes .strm, .nfo, poster.jpg, and fanart.jpg files to the Emby directory structure.
func ExportEmbyMedia(embyDir, publicURL, strmToken, code string, doc nfo.Movie, videos []pan.File, poster, fanart []byte) error {
	stem := nfo.FileStem(code)
	destDir := EmbyMovieDir(embyDir, code)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create emby directory %s: %w", destDir, err)
	}

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
	if len(videos) == 1 {
		strmPath := filepath.Join(destDir, stem+".strm")
		if err := os.WriteFile(strmPath, STRMContent(publicURL, videos[0].ID, strmToken), 0o644); err != nil {
			return fmt.Errorf("write strm file: %w", err)
		}
	} else if len(videos) > 1 {
		for i, v := range videos {
			strmPath := filepath.Join(destDir, fmt.Sprintf("%s-cd%d.strm", stem, i+1))
			if err := os.WriteFile(strmPath, STRMContent(publicURL, v.ID, strmToken), 0o644); err != nil {
				return fmt.Errorf("write strm file: %w", err)
			}
		}
	}

	return nil
}

// MediaNotifier receives notifications when exported media directories are written or updated.
type MediaNotifier interface {
	NotifyUpdated(localPath string)
}

// ExportLocalMovie exports an already-scraped ent.Movie record and cached artwork to the Emby directory if missing.
func ExportLocalMovie(embyDir, publicURL, strmToken string, record *ent.Movie, images *mediaimage.Cache, notifiers ...MediaNotifier) error {
	if record == nil || record.Code == "" {
		return nil
	}

	destDir := EmbyMovieDir(embyDir, record.Code)
	stem := nfo.FileStem(record.Code)
	nfoPath := filepath.Join(destDir, stem+".nfo")
	posterPath := filepath.Join(destDir, "poster.jpg")
	strmPath := filepath.Join(destDir, stem+".strm")

	// If .nfo and poster.jpg already exist, and at least one strm exists, we don't need to re-write.
	_, nfoErr := os.Stat(nfoPath)
	_, posterErr := os.Stat(posterPath)
	_, strmErr := os.Stat(strmPath)
	if nfoErr == nil && posterErr == nil && (strmErr == nil || len(record.Edges.Files) > 1) {
		return nil
	}

	doc := MovieNFO(record)
	videos := make([]pan.File, 0, len(record.Edges.Files))
	for _, f := range record.Edges.Files {
		videos = append(videos, pan.File{ID: f.FileID, Name: f.Name, Size: f.Size, PickCode: f.PickCode})
	}

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
