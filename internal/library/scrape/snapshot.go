package scrape

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

// Sidecar records the identity and hash of an NFO, poster, or fanart sidecar.
type Sidecar struct {
	Name string `json:"name"`
	SHA1 string `json:"sha1"`
}

// DirectorySnapshot records the sidecar state of a single media directory.
type DirectorySnapshot struct {
	ID     string  `json:"id"`
	NFO    Sidecar `json:"nfo"`
	Poster Sidecar `json:"poster"`
	Fanart Sidecar `json:"fanart"`
}

// NewDirectorySnapshot creates a DirectorySnapshot from pan.File entries.
func NewDirectorySnapshot(id string, nfo, poster, fanart pan.File) DirectorySnapshot {
	return DirectorySnapshot{
		ID:     id,
		NFO:    Sidecar{Name: nfo.Name, SHA1: nfo.SHA1},
		Poster: Sidecar{Name: poster.Name, SHA1: poster.SHA1},
		Fanart: Sidecar{Name: fanart.Name, SHA1: fanart.SHA1},
	}
}

// Snapshot records the exact videos and sidecars handled by a completed cover job.
// Later scans compare their directory listings without downloading unchanged NFOs or images.
type Snapshot struct {
	Videos      string              `json:"videos"`
	Directories []DirectorySnapshot `json:"directories,omitempty"`
	LocalExport bool                `json:"local_export,omitempty"`
}

// ObservedFile represents a file observed during a scan in a directory.
type ObservedFile struct {
	Sidecar
	VideoID string
}

// ObservedDirectory is a slice of observed files in a directory.
type ObservedDirectory []ObservedFile

// Sidecar finds an observed sidecar by name (case-insensitive).
func (d ObservedDirectory) Sidecar(name string) (Sidecar, bool) {
	for _, entry := range d {
		if strings.EqualFold(entry.Name, name) {
			return entry.Sidecar, true
		}
	}
	return Sidecar{}, false
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
		file := ObservedFile{Sidecar: Sidecar{Name: entry.Name, SHA1: entry.SHA1}}
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

// VideosMatch checks if the snapshot matches the current movie video files.
func (snapshot Snapshot) VideosMatch(record *ent.Movie) bool {
	files := make([]pan.File, 0, len(record.Edges.Files))
	for _, entry := range record.Edges.Files {
		files = append(files, pan.File{ID: entry.FileID, ParentID: entry.ParentID, Name: entry.Name, SHA1: entry.Sha1, Size: entry.Size})
	}
	return snapshot.Videos == VideoFingerprint(files)
}

// Matches checks if the snapshot matches the current movie files and directory observations.
func (snapshot Snapshot) Matches(record *ent.Movie, directories DirectoryObservations) bool {
	if !snapshot.VideosMatch(record) {
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
		for _, sidecar := range []Sidecar{saved.NFO, saved.Poster, saved.Fanart} {
			entry, found := directory.Sidecar(sidecar.Name)
			if !found || entry.SHA1 == "" || !strings.EqualFold(entry.SHA1, sidecar.SHA1) {
				return false
			}
		}
	}
	return true
}

// CompletedMetadataSnapshots loads the latest completed cover job snapshot for each movie ID.
func CompletedMetadataSnapshots(ctx context.Context, database *ent.Client, source domain.LibrarySource, movieIDs []int) (map[int]Snapshot, error) {
	result := make(map[int]Snapshot)
	for start := 0; start < len(movieIDs); start += 500 {
		ids := make([]any, 0, 500)
		for _, id := range movieIDs[start:min(start+500, len(movieIDs))] {
			ids = append(ids, id)
		}
		table := sql.Table(task.Table)
		latest := sql.Select(sql.Max(table.C(task.FieldID))).From(table).Where(sql.And(
			sql.EQ(table.C(task.FieldType), tasks.KindCover.String()), sql.EQ(table.C(task.FieldStatus), task.StatusDone),
			sqljson.ValueIn(table.C(task.FieldPayload), ids, sqljson.Path("movie_id")),
			sqljson.ValueEQ(table.C(task.FieldPayload), source.AccountID, sqljson.Path("source", "account_id")),
			sqljson.ValueEQ(table.C(task.FieldPayload), source.Directory.ID, sqljson.Path("source", "directory", "id")),
		)).GroupBy(tasks.JSONExtract(table.C(task.FieldPayload), "movie_id"))
		var records []struct {
			MovieID  int     `json:"movie_id"`
			Snapshot *string `json:"snapshot"`
		}
		err := database.Task.Query().Where(func(s *sql.Selector) {
			s.Where(sql.In(s.C(task.FieldID), latest))
			s.Select(sql.As(tasks.JSONExtract(s.C(task.FieldPayload), "movie_id"), "movie_id"),
				sql.As(tasks.JSONExtract(s.C(task.FieldPayload), "snapshot"), "snapshot"))
		}).Select(task.FieldID).Scan(ctx, &records)
		if err != nil {
			return nil, fmt.Errorf("load completed metadata snapshots: %w", err)
		}
		for _, record := range records {
			if record.Snapshot != nil {
				var snapshot Snapshot
				if err := json.Unmarshal([]byte(*record.Snapshot), &snapshot); err != nil {
					return nil, fmt.Errorf("decode completed metadata snapshot: %w", err)
				}
				result[record.MovieID] = snapshot
			}
		}
	}
	return result, nil
}

// MovieArtwork extracts the artwork URLs from an indexed ent.Movie record.
func MovieArtwork(record *ent.Movie) mediaimage.Artwork {
	artwork := mediaimage.Artwork{Poster: domain.ValueOrZero(record.Poster), Thumbnail: domain.ValueOrZero(record.Cover)}
	if len(record.Fanarts) > 0 {
		artwork.Fanart = record.Fanarts[0]
	}
	return artwork
}
