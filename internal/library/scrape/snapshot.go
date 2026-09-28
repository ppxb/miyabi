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

// NewDirectorySnapshot creates a DirectorySnapshot from pan.File entries.
func NewDirectorySnapshot(id string, nfo, poster, fanart pan.File) domain.DirectorySnapshot {
	return domain.DirectorySnapshot{
		ID:     id,
		NFO:    domain.Sidecar{Name: nfo.Name, SHA1: nfo.SHA1},
		Poster: domain.Sidecar{Name: poster.Name, SHA1: poster.SHA1},
		Fanart: domain.Sidecar{Name: fanart.Name, SHA1: fanart.SHA1},
	}
}

// ObservedFile represents a file observed during a scan in a directory.
type ObservedFile struct {
	domain.Sidecar
	VideoID string
}

// ObservedDirectory is a slice of observed files in a directory.
type ObservedDirectory []ObservedFile

// Sidecar finds an observed sidecar by name (case-insensitive).
func (d ObservedDirectory) Sidecar(name string) (domain.Sidecar, bool) {
	for _, entry := range d {
		if strings.EqualFold(entry.Name, name) {
			return entry.Sidecar, true
		}
	}
	return domain.Sidecar{}, false
}

// DirectoryObservations maps directory IDs to their observed files.
type DirectoryObservations map[string]ObservedDirectory

// Add records observations for non-directory files in a directory.
func (observed DirectoryObservations) Add(id string, files []pan.File) {
	directory := observed[id]
	fileCount := 0
	for _, entry := range files {
		if !entry.IsDirectory {
			fileCount++
		}
	}
	directory = slices.Grow(directory, fileCount)
	for _, entry := range files {
		if entry.IsDirectory {
			continue
		}
		file := ObservedFile{Sidecar: domain.Sidecar{Name: entry.Name, SHA1: entry.SHA1}}
		if !entry.IsDirectory && domain.IsVideo(entry.Name) && entry.Size >= domain.MinVideoSize {
			file.VideoID = entry.ID
		}
		directory = append(directory, file)
	}
	observed[id] = directory
}

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
func SnapshotMatches(record *ent.Movie, source domain.LibrarySource, directories DirectoryObservations) bool {
	snapshot := record.MetadataSnapshot
	if snapshot == nil || snapshot.AccountID != source.AccountID || snapshot.DirectoryID != source.Directory.ID {
		return false
	}
	files := make([]pan.File, 0, len(record.Edges.Files))
	for _, entry := range record.Edges.Files {
		files = append(files, pan.File{ID: entry.FileID, ParentID: entry.ParentID, Name: entry.Name, SHA1: entry.Sha1, Size: entry.Size})
	}
	if snapshot.Videos != VideoFingerprint(files) {
		return false
	}
	if snapshot.LocalExport {
		return true
	}
	if len(snapshot.Directories) == 0 {
		return false
	}
	ids := make(map[string]bool, len(record.Edges.Files))
	for _, entry := range record.Edges.Files {
		ids[entry.FileID] = true
	}

	for _, saved := range snapshot.Directories {
		directory := directories[saved.ID]
		shared := slices.ContainsFunc(directory, func(entry ObservedFile) bool { return entry.VideoID != "" && !ids[entry.VideoID] })
		nfoFile, found := FindNFO(record.Code, shared, directory, func(entry ObservedFile) string { return entry.Name })
		if !found || !strings.EqualFold(nfoFile.Name, saved.NFO.Name) {
			return false
		}
		for _, sidecar := range []domain.Sidecar{saved.NFO, saved.Poster, saved.Fanart} {
			entry, found := directory.Sidecar(sidecar.Name)
			if !found || entry.SHA1 == "" || !strings.EqualFold(entry.SHA1, sidecar.SHA1) {
				return false
			}
		}
	}
	return true
}

// MovieArtwork extracts the artwork URLs from an indexed ent.Movie record.
func MovieArtwork(record *ent.Movie) mediaimage.Artwork {
	artwork := mediaimage.Artwork{Poster: domain.ValueOrZero(record.Poster), Thumbnail: domain.ValueOrZero(record.Cover)}
	if len(record.Fanarts) > 0 {
		artwork.Fanart = record.Fanarts[0]
	}
	return artwork
}
