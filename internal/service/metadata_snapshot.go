package service

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
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/pan"
)

// A completed cover job records the exact videos and sidecars it handled.
// Later scans compare their existing directory listings without downloading
// unchanged NFOs or images. The snapshot remains a rebuildable local index.
type metadataSnapshot struct {
	Videos      string                      `json:"videos"`
	Directories []metadataDirectorySnapshot `json:"directories"`
}

type metadataSidecar struct {
	Name string `json:"name"`
	SHA1 string `json:"sha1"`
}

type metadataDirectorySnapshot struct {
	ID     string          `json:"id"`
	NFO    metadataSidecar `json:"nfo"`
	Poster metadataSidecar `json:"poster"`
	Fanart metadataSidecar `json:"fanart"`
}

func directorySnapshot(id string, nfo, poster, fanart pan.File) metadataDirectorySnapshot {
	return metadataDirectorySnapshot{ID: id,
		NFO:    metadataSidecar{Name: nfo.Name, SHA1: nfo.SHA1},
		Poster: metadataSidecar{Name: poster.Name, SHA1: poster.SHA1},
		Fanart: metadataSidecar{Name: fanart.Name, SHA1: fanart.SHA1},
	}
}

func videoFingerprint(files []pan.File) string {
	parts := make([]string, 0, len(files))
	for _, entry := range files {
		parts = append(parts, fmt.Sprintf("%q %q %q %q %d", entry.ID, entry.ParentID, entry.Name, strings.ToUpper(entry.SHA1), entry.Size))
	}
	slices.Sort(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func (snapshot metadataSnapshot) matches(record *ent.Movie, directories scanObservations) bool {
	files := make([]pan.File, 0, len(record.Edges.Files))
	ids := make(map[string]bool, len(record.Edges.Files))
	for _, entry := range record.Edges.Files {
		files = append(files, pan.File{ID: entry.FileID, ParentID: entry.ParentID, Name: entry.Name, SHA1: entry.Sha1, Size: entry.Size})
		ids[entry.FileID] = true
	}
	if snapshot.Videos != videoFingerprint(files) || len(snapshot.Directories) == 0 {
		return false
	}
	for _, saved := range snapshot.Directories {
		directory := directories[saved.ID]
		shared := slices.ContainsFunc(directory, func(entry observedFile) bool { return entry.videoID != "" && !ids[entry.videoID] })
		nfo, found := findNFO(record.Code, shared, directory, func(entry observedFile) string { return entry.Name })
		if !found || !strings.EqualFold(nfo.Name, saved.NFO.Name) {
			return false
		}
		for _, sidecar := range []metadataSidecar{saved.NFO, saved.Poster, saved.Fanart} {
			entry, found := directory.sidecar(sidecar.Name)
			if !found || entry.SHA1 == "" || !strings.EqualFold(entry.SHA1, sidecar.SHA1) {
				return false
			}
		}
	}
	return true
}

func completedMetadataSnapshots(ctx context.Context, database *ent.Client, source LibrarySource, movieIDs []int) (map[int]metadataSnapshot, error) {
	result := make(map[int]metadataSnapshot)
	for start := 0; start < len(movieIDs); start += 500 {
		ids := make([]any, 0, 500)
		for _, id := range movieIDs[start:min(start+500, len(movieIDs))] {
			ids = append(ids, id)
		}
		table := sql.Table(task.Table)
		latest := sql.Select(sql.Max(table.C(task.FieldID))).From(table).Where(sql.And(
			sql.EQ(table.C(task.FieldType), "cover"), sql.EQ(table.C(task.FieldStatus), task.StatusDone),
			sqljson.ValueIn(table.C(task.FieldPayload), ids, sqljson.Path("movie_id")),
			sqljson.ValueEQ(table.C(task.FieldPayload), source.AccountID, sqljson.Path("source", "account_id")),
			sqljson.ValueEQ(table.C(task.FieldPayload), source.Directory.ID, sqljson.Path("source", "directory", "id")),
		)).GroupBy("json_extract(" + table.C(task.FieldPayload) + ", '$.movie_id')")
		var records []struct {
			MovieID  int     `json:"movie_id"`
			Snapshot *string `json:"snapshot"`
		}
		err := database.Task.Query().Where(func(s *sql.Selector) {
			s.Where(sql.In(s.C(task.FieldID), latest))
			s.Select(sql.As("json_extract("+s.C(task.FieldPayload)+", '$.movie_id')", "movie_id"),
				sql.As("json_extract("+s.C(task.FieldPayload)+", '$.snapshot')", "snapshot"))
		}).Select(task.FieldID).Scan(ctx, &records)
		if err != nil {
			return nil, fmt.Errorf("load completed metadata snapshots: %w", err)
		}
		for _, record := range records {
			if record.Snapshot != nil {
				var snapshot metadataSnapshot
				if err := json.Unmarshal([]byte(*record.Snapshot), &snapshot); err != nil {
					return nil, fmt.Errorf("decode completed metadata snapshot: %w", err)
				}
				result[record.MovieID] = snapshot
			}
		}
	}
	return result, nil
}

func movieArtwork(record *ent.Movie) mediaimage.Artwork {
	artwork := mediaimage.Artwork{Poster: valueOrZero(record.Poster), Thumbnail: valueOrZero(record.Cover)}
	if len(record.Fanarts) > 0 {
		artwork.Fanart = record.Fanarts[0]
	}
	return artwork
}
