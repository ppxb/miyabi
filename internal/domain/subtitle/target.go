package subtitle

import (
	"context"
	"path/filepath"
	"strings"
)

// Reader reads files from the mounted 115 directory.
type Reader interface {
	Read(ctx context.Context, pickCode string, limit int64) ([]byte, error)
}

// Target locates the exported .strm file a movie's subtitles accompany.
type Target struct {
	Dir  string
	Stem string
	Code string
	// Uncensored selects subtitles timed for uncensored cuts.
	Uncensored bool
	// HardSubtitled videos already show Chinese subtitles; no online search is made.
	HardSubtitled bool
}

// Path names a subtitle so Emby attaches it to the .strm and reads its
// language: <stem>[.<version>].<language>.<format>.
func (target Target) Path(kind Kind) string {
	parts := []string{target.Stem}
	if kind.Version != VersionStandard && kind.Version != "" {
		parts = append(parts, string(kind.Version))
	}
	parts = append(parts, string(kind.Language), kind.Format)
	return filepath.Join(target.Dir, strings.Join(parts, "."))
}

// Kind groups interchangeable subtitles. Emby lists one track per kind, so a
// movie keeps at most one subtitle of each.
type Kind struct {
	Language Language
	Version  VersionTag
	Format   string
}
