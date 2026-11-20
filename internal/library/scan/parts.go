package scan

import (
	"context"
	"path"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/pan"
)

// directoryPartCode requires an explicit directory number and a complete A, B,
// ... sequence with a common filename stem. A letter alone is not a part marker.
func directoryPartCode(directory string, videos []Video, sidecars []pan.File) string {
	code := codeid.Normalize(path.Base(directory))
	if !codeid.Valid(code) {
		return ""
	}
	var stem string
	parts := make(map[byte]bool)
	for _, video := range videos {
		if !canIdentifyVideo(video.File) {
			continue
		}
		name := strings.ToUpper(strings.TrimSuffix(video.Name, path.Ext(video.Name)))
		if name == "" {
			return ""
		}
		part := name[len(name)-1]
		prefix := name[:len(name)-1]
		base := strings.TrimRight(prefix, "-_. ")
		if part < 'A' || part > 'Z' || parts[part] ||
			(base != "" && !codeid.IsFormatEquivalent(base, code)) ||
			(len(parts) > 0 && prefix != stem) {
			return ""
		}
		stem, parts[part] = prefix, true
	}
	if len(parts) < 2 {
		return ""
	}
	for i := range len(parts) {
		if !parts['A'+byte(i)] {
			return ""
		}
	}
	// Explicit per-release sidecars outweigh the directory naming convention.
	for _, sidecar := range sidecars {
		if other, ok := codeid.Parse(sidecar.Name); ok && !codeid.IsEquivalent(code, other) {
			return ""
		}
	}
	return code
}

// resolveDirectoryParts applies directory evidence only to unconfirmed names.
// Existing manual or scraped identities can veto the whole group, so a genuine
// letter variant cannot donate its siblings to a different movie.
func resolveDirectoryParts(ctx context.Context, db *ent.Client, accountID, directory string, videos []Video, sidecars []pan.File) error {
	code := directoryPartCode(directory, videos, sidecars)
	if code == "" {
		return nil
	}
	ids := make([]string, 0, len(videos))
	byID := make(map[string]pan.File)
	for _, video := range videos {
		if canIdentifyVideo(video.File) {
			ids = append(ids, video.ID)
			byID[video.ID] = video.File
		}
	}
	records, err := db.File.Query().Where(file.AccountIDEQ(accountID), file.FileIDIn(ids...),
		file.HasMovieWith(movie.Or(movie.ManualCodeNEQ(""), movie.ScrapeStatusEQ(movie.ScrapeStatusDone)))).
		Select(file.FieldID, file.FieldFileID, file.FieldName, file.FieldSize, file.FieldSha1, file.FieldMovieID).
		WithMovie(func(q *ent.MovieQuery) {
			q.Select(movie.FieldCode, movie.FieldManualCode, movie.FieldScrapeStatus)
		}).All(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if directoryPartConflict(code, byID[record.FileID], record) {
			return nil
		}
	}
	// At most 26 parts fit in one indexing chunk. Put them before auxiliary
	// files so a concurrently confirmed identity can reject the entire group.
	next := 0
	for i := range videos {
		if canIdentifyVideo(videos[i].File) {
			videos[i].Code, videos[i].DirectoryCode = code, code
			videos[next], videos[i] = videos[i], videos[next]
			next++
		}
	}
	return nil
}

func directoryPartConflict(code string, current pan.File, previous *ent.File) bool {
	if previous == nil || previous.Edges.Movie == nil {
		return false
	}
	film := previous.Edges.Movie
	confirmed := film.ManualCode != "" ||
		(film.ScrapeStatus == movie.ScrapeStatusDone &&
			previous.Name == current.Name && previous.Size == current.Size && previous.Sha1 == current.SHA1)
	return confirmed && !codeid.IsEquivalent(film.Code, code)
}
