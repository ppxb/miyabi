package scan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/export"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/nfo"
)

const localScanBatchSize = 32

type localMedia struct {
	path, name, code                string
	size                            int64
	nfoPath, posterPath, fanartPath string
	subPaths                        []string
	rel, fileID                     string
	linked                          bool // fileID was read from a Miyabi .strm URL
	document                        *nfo.Movie
	artwork                         mediaimage.Artwork
}

// identifyMedia derives the file identity; a Miyabi .strm names the file it plays.
func identifyMedia(rootDir string, media *localMedia) error {
	if domain.IsSTRM(media.name) {
		body, _ := os.ReadFile(media.path)
		media.fileID = export.ParseSTRMFileID(string(body))
		media.linked = media.fileID != ""
	}
	rel, err := filepath.Rel(rootDir, media.path)
	if err != nil {
		return fmt.Errorf("compute relative path for %s: %w", media.path, err)
	}
	media.rel = rel
	if media.fileID == "" {
		// Scan normalizes the root first, so the absolute path distinguishes
		// identical relative names in different libraries without aliasing rescans.
		hash := sha256.Sum256([]byte(media.path))
		media.fileID = domain.LocalFileIDPrefix + hex.EncodeToString(hash[:16])
	}
	return nil
}

// settledExports finds Miyabi's own exports of completed movies with cached
// artwork. Importing them again only rewrites unchanged metadata and re-encodes
// the exported JPEGs into new cache entries, which the next export writes back.
func (s *LocalScanner) settledExports(ctx context.Context, batch []localMedia) (map[string]bool, error) {
	if s.images == nil {
		return nil, nil
	}
	var linked []string
	for _, media := range batch {
		if media.linked {
			linked = append(linked, media.fileID)
		}
	}
	if len(linked) == 0 {
		return nil, nil
	}
	records, err := s.db.File.Query().
		Where(file.FileIDIn(linked...), file.HasMovieWith(movie.ScrapeStatusEQ(movie.ScrapeStatusDone))).
		WithMovie().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("find completed exports: %w", err)
	}
	settled := make(map[string]bool, len(records))
	for _, record := range records {
		// Artwork outside the cache, or missing from it, is restored by the import.
		if cached, err := s.images.Exists(scrape.MovieArtwork(record.Edges.Movie)); err == nil && cached {
			settled[record.FileID] = true
		}
	}
	return settled, nil
}

// Prepare sidecars and artwork before taking SQLite's writer lock. Only one
// bounded batch is prepared at a time; decoded images are not kept in memory.
func (s *LocalScanner) prepareMedia(ctx context.Context, media *localMedia) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if media.nfoPath != "" {
		body, err := os.ReadFile(media.nfoPath)
		if err != nil {
			slog.WarnContext(ctx, "failed to read local NFO", "path", media.nfoPath, "error", err)
		} else if doc, err := nfo.Decode(body); err != nil {
			slog.WarnContext(ctx, "failed to decode local NFO", "path", media.nfoPath, "error", err)
		} else {
			doc.Code = codeid.Normalize(doc.Code)
			if doc.Code == "" {
				doc.Code = media.code
			}
			media.document = &doc
		}
	}
	if s.images != nil && media.posterPath != "" {
		poster, _ := os.ReadFile(media.posterPath)
		if len(poster) > 0 {
			var fanart []byte
			if media.fanartPath != "" {
				fanart, _ = os.ReadFile(media.fanartPath)
			}
			var artwork mediaimage.Artwork
			var err error
			if len(fanart) > 0 {
				artwork, err = s.images.Restore(poster, fanart)
			} else {
				artwork, err = s.images.Restore(poster, poster)
			}
			if err == nil {
				media.artwork = artwork
			}
		}
	}
	return ctx.Err()
}

func (s *LocalScanner) ingestBatch(ctx context.Context, rootDir string, batch []localMedia, result *LocalScanResult) error {
	codes, fileIDs := make([]string, len(batch)), make([]string, len(batch))
	for i := range batch {
		if err := identifyMedia(rootDir, &batch[i]); err != nil {
			return err
		}
		codes[i], fileIDs[i] = batch[i].code, batch[i].fileID
	}
	settled, err := s.settledExports(ctx, batch)
	if err != nil {
		return err
	}
	if s.images != nil {
		for _, media := range batch {
			if media.posterPath != "" && !settled[media.fileID] {
				if err := s.images.LockArtwork(ctx); err != nil {
					return err
				}
				defer s.images.UnlockArtwork()
				break
			}
		}
	}
	for i := range batch {
		if settled[batch[i].fileID] {
			continue
		}
		if err := s.prepareMedia(ctx, &batch[i]); err != nil {
			return fmt.Errorf("prepare %s: %w", batch[i].path, err)
		}
	}
	var added, nfoRead int
	err = ent.WithTx(ctx, s.db, func(tx *ent.Tx) error {
		records, err := tx.File.Query().Where(file.FileIDIn(fileIDs...)).All(ctx)
		if err != nil {
			return err
		}
		files := make(map[string]*ent.File, len(records))
		var associatedIDs []int
		for _, record := range records {
			files[record.FileID] = record
			if record.MovieID != nil {
				associatedIDs = append(associatedIDs, *record.MovieID)
			}
		}
		matcher, err := loadMovieMatcher(ctx, tx, codes, associatedIDs...)
		if err != nil {
			return err
		}
		manualMovies := make(map[int]bool)
		for _, group := range matcher {
			for _, record := range group {
				manualMovies[record.ID] = record.ManualCode != ""
			}
		}
		directories := make(map[string]bool)
		for _, media := range batch {
			if err := ctx.Err(); err != nil {
				return err
			}
			existing := files[media.fileID]
			// A .strm exported for a local file names that file's row; it must not
			// move the row onto the .strm, or local playback reads the .strm instead.
			referencesLocal := media.linked && strings.HasPrefix(media.fileID, domain.LocalFileIDPrefix)
			if existing != nil && existing.AccountID == domain.LocalAccountID && !referencesLocal &&
				(existing.Name != media.name || existing.Size != media.size || existing.Path != media.path) {
				updated, err := tx.File.UpdateOneID(existing.ID).SetName(media.name).SetSize(media.size).SetPath(media.path).Save(ctx)
				if err != nil {
					return err
				}
				existing, files[media.fileID] = updated, updated
			}
			// A stale exported NFO must not undo a manual correction or reassign its files.
			if existing != nil && existing.MovieID != nil && manualMovies[*existing.MovieID] {
				continue
			}
			// Unchanged Miyabi output for a completed movie: its sidecars were not
			// read and Emby already has the directory.
			if settled[media.fileID] && existing != nil && existing.MovieID != nil {
				for _, subPath := range media.subPaths {
					if err := indexLocalSubtitle(ctx, tx, *existing.MovieID, subPath); err != nil {
						return err
					}
				}
				continue
			}
			record := matcher.find(media.code)
			if record == nil {
				record, err = tx.Movie.Create().SetCode(media.code).Save(ctx)
				if err != nil {
					return err
				}
				matcher.add(record)
				added++
			}
			// An old exported NFO does not mean a failed refresh has recovered.
			if media.document != nil && record.ScrapeStatus != movie.ScrapeStatusFailed {
				nfoRead++
				var current *ent.Movie
				if err := scrape.SaveMovieMetadata(ctx, tx, record.ID, *media.document); err != nil {
					slog.WarnContext(ctx, "failed to save movie metadata from local NFO", "path", media.nfoPath, "error", err)
					// Even a rejected NFO may have changed fields before a later write failed.
					current, err = tx.Movie.Get(ctx, record.ID)
					if err != nil {
						return err
					}
				} else {
					current, err = tx.Movie.UpdateOneID(record.ID).SetScrapeStatus(movie.ScrapeStatusDone).Save(ctx)
					if err != nil {
						return err
					}
				}
				matcher.remove(record)
				matcher.add(current)
				record = current
			}
			if media.document == nil && record.ScrapeStatus == movie.ScrapeStatusPending {
				result.addPending(record.ID)
			}
			if media.artwork != (mediaimage.Artwork{}) {
				if err := tx.Movie.UpdateOneID(record.ID).SetPoster(media.artwork.Poster).
					SetCover(media.artwork.Thumbnail).SetFanarts([]string{media.artwork.Fanart}).Exec(ctx); err != nil {
					return err
				}
			}
			if existing == nil {
				created, err := tx.File.Create().SetFileID(media.fileID).SetName(media.name).SetSize(media.size).
					SetAccountID(domain.LocalAccountID).SetRootID(domain.LocalAccountID).
					SetPath(media.path).SetMovieID(record.ID).Save(ctx)
				if err != nil {
					return err
				}
				files[media.fileID] = created
			} else if existing.MovieID == nil || *existing.MovieID != record.ID {
				updated, err := tx.File.UpdateOneID(existing.ID).SetMovieID(record.ID).Save(ctx)
				if err != nil {
					return err
				}
				files[media.fileID] = updated
			}
			for _, subPath := range media.subPaths {
				if err := indexLocalSubtitle(ctx, tx, record.ID, subPath); err != nil {
					return err
				}
			}
			directories[filepath.Dir(media.path)] = true
		}
		if s.notifier != nil {
			for dir := range directories {
				if err := s.notifier.NotifyUpdatedTx(ctx, tx, dir); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("ingest local batch starting at %s: %w", batch[0].path, err)
	}
	result.MoviesAdded += added
	result.NFORead += nfoRead
	return nil
}
