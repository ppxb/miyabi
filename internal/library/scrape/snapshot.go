package scrape

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/pan"
)

// VideoFingerprint computes a deterministic hash of a movie's video files.
func VideoFingerprint(files []pan.File) string {
	parts := make([]string, 0, len(files))
	for _, entry := range files {
		parts = append(parts, fmt.Sprintf("%q %q %q %q %d", entry.ID, entry.ParentID, entry.Name, strings.ToUpper(entry.SHA1), entry.Size))
	}
	slices.Sort(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

// SnapshotMatches compares the movie's saved export with this source's current files.
func SnapshotMatches(record *ent.Movie, source domain.LibrarySource) bool {
	snapshot := record.MetadataSnapshot
	if snapshot == nil || snapshot.AccountID != source.AccountID || snapshot.DirectoryID != source.Directory.ID {
		return false
	}
	files := make([]pan.File, 0, len(record.Edges.Files))
	for _, entry := range record.Edges.Files {
		files = append(files, pan.File{ID: entry.FileID, ParentID: entry.ParentID, Name: entry.Name, SHA1: entry.Sha1, Size: entry.Size})
	}
	return snapshot.Videos == VideoFingerprint(files)
}

// MovieArtwork extracts the artwork URLs from an indexed ent.Movie record.
func MovieArtwork(record *ent.Movie) mediaimage.Artwork {
	artwork := mediaimage.Artwork{Poster: domain.ValueOrZero(record.Poster), Thumbnail: domain.ValueOrZero(record.Cover)}
	if len(record.Fanarts) > 0 {
		artwork.Fanart = record.Fanarts[0]
	}
	return artwork
}
