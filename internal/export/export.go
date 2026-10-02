package export

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/nfo"
)

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

// EmbyMovieDir is the directory holding a movie's exported Emby files,
// bucketed by catalogue prefix: <embyDir>/<prefix>/<safe-stem>.
// embyDir must come from the resolved export configuration.
func EmbyMovieDir(embyDir, code string) string {
	return filepath.Join(embyDir, nfo.FileStem(codeid.Prefix(code)), nfo.FileStem(code))
}

// STRMContent is the body of a .strm file: the relay URL that resolves the
// 115 video to a fresh stream whenever Emby plays it.
// publicURL must come from the resolved export configuration.
func STRMContent(publicURL, fileID, strmToken string) []byte {
	publicURL = strings.TrimRight(strings.TrimSpace(publicURL), "/")
	content := publicURL + STRMPlayPath + fileID
	if strmToken != "" {
		content += "?token=" + url.QueryEscape(strmToken)
	}
	return []byte(content + "\n")
}

// RewriteSTRM walks the Emby export directory and updates all existing .strm files
// to use the active publicURL and strmToken. It respects cancellation via ctx.
// It returns the count of rewritten files.
func RewriteSTRM(ctx context.Context, embyDir, publicURL, strmToken string) (int, error) {
	if err := (Config{EmbyDir: embyDir, PublicURL: publicURL}).Validate(); err != nil {
		return 0, err
	}

	rewritten := 0
	err := filepath.WalkDir(embyDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".strm") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read STRM %s: %w", path, err)
		}
		fileID := ParseSTRMFileID(string(data))
		if fileID == "" {
			return nil
		}
		newContent := STRMContent(publicURL, fileID, strmToken)
		if !bytes.Equal(bytes.TrimSpace(data), bytes.TrimSpace(newContent)) {
			if err := os.WriteFile(path, newContent, 0o644); err != nil {
				return fmt.Errorf("rewrite STRM %s: %w", path, err)
			}
			rewritten++
		}
		return nil
	})
	return rewritten, err
}

// MediaNotifier receives notifications when exported media directories are written or updated.
type MediaNotifier interface {
	NotifyUpdated(ctx context.Context, localPath string) error
	NotifyUpdatedTx(ctx context.Context, tx *ent.Tx, localPath string) error
}
