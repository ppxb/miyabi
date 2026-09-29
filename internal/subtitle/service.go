package subtitle

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	subtitlemeta "github.com/ppxb/miyabi/internal/domain/subtitle"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/subtitle"
)

const (
	// MaxTracks bounds the subtitles exported per movie, one of each subtitlemeta.Kind.
	MaxTracks = 3
	// maxDownloads bounds the subtitles downloaded per export: a candidate
	// without a language hint may turn out to repeat an exported kind.
	maxDownloads = 8
)

// Service exports movie subtitles beside their Emby .strm files.
type Service struct {
	db     *ent.Client
	finder *Finder
}

func NewService(db *ent.Client, finder *Finder) *Service {
	return &Service{db: db, finder: finder}
}

// Export writes a movie's subtitles beside its .strm file and returns how many
// files it wrote. Existing exports are reused; online subtitles fill the
// remaining kinds up to MaxTracks.
func (s *Service) Export(ctx context.Context, movieID int, target subtitlemeta.Target) (int, error) {
	tracks, err := s.db.Subtitle.Query().Where(subtitle.MovieIDEQ(movieID)).Order(ent.Asc(subtitle.FieldID)).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("list movie subtitles: %w", err)
	}
	exported := make(map[subtitlemeta.Kind]bool)
	var pending []*ent.Subtitle
	for _, track := range tracks {
		if filepath.Dir(track.StoragePath) == target.Dir && fileExists(track.StoragePath) {
			exported[trackKind(track)] = true
		} else {
			pending = append(pending, track)
		}
	}
	written := 0
	var errs []error
	for _, track := range pending {
		if track.SourceURL != "" {
			// The exported file was removed, or predates exports beside the .strm.
			// Drop the record and any stale copy; the online search replaces it.
			if kind := trackKind(track); kind.Format != "" && !exported[kind] {
				_ = os.Remove(target.Path(kind))
			}
		}
		errs = append(errs, s.db.Subtitle.DeleteOne(track).Exec(ctx))
	}
	if !target.HardSubtitled && len(exported) < MaxTracks {
		count, err := s.exportOnline(ctx, movieID, target, exported)
		written += count
		errs = append(errs, err)
	}
	return written, errors.Join(errs...)
}

type download struct {
	body     []byte
	language subtitlemeta.Language
	err      error
}

// exportOnline fills the remaining kinds in two passes: first a language or
// cut the movie lacks, then another format of one it has.
func (s *Service) exportOnline(ctx context.Context, movieID int, target subtitlemeta.Target, exported map[subtitlemeta.Kind]bool) (int, error) {
	candidates := s.finder.Search(ctx, target.Code, target.Uncensored)
	downloads := make(map[string]download)
	written := make(map[[sha256.Size]byte]bool)
	for _, distinctLanguage := range []bool{true, false} {
		for _, candidate := range candidates {
			if len(exported) >= MaxTracks {
				return len(written), nil
			}
			if candidate.Language != subtitlemeta.LangUnknown && !wanted(candidate.Kind(), exported, distinctLanguage) {
				continue
			}
			result, fetched := downloads[candidate.URL]
			if !fetched {
				if len(downloads) >= maxDownloads {
					continue
				}
				result.body, result.language, result.err = s.finder.Download(ctx, candidate)
				downloads[candidate.URL] = result
			}
			if result.err != nil {
				continue
			}
			candidate.Language = result.language
			digest := sha256.Sum256(result.body)
			if !wanted(candidate.Kind(), exported, distinctLanguage) || written[digest] {
				continue
			}
			destination := target.Path(candidate.Kind())
			if err := writeFile(destination, result.body); err != nil {
				return len(written), err
			}
			exported[candidate.Kind()] = true
			written[digest] = true
			if err := s.db.Subtitle.Create().SetMovieID(movieID).SetName(filepath.Base(destination)).
				SetLanguage(string(candidate.Language)).SetFormat(candidate.Format).SetVersionTag(string(candidate.Version)).
				SetSource(candidate.Provider).SetSourceURL(candidate.URL).SetStoragePath(destination).Exec(ctx); err != nil {
				return len(written), fmt.Errorf("record online subtitle: %w", err)
			}
		}
	}
	return len(written), nil
}

// wanted reports whether kind adds a track. With distinctLanguage, the movie
// must also lack any format of the kind's language and cut.
func wanted(kind subtitlemeta.Kind, exported map[subtitlemeta.Kind]bool, distinctLanguage bool) bool {
	if exported[kind] {
		return false
	}
	if distinctLanguage {
		for other := range exported {
			if other.Language == kind.Language && other.Version == kind.Version {
				return false
			}
		}
	}
	return true
}

func trackKind(track *ent.Subtitle) subtitlemeta.Kind {
	return subtitlemeta.Kind{Language: subtitlemeta.Language(track.Language), Version: subtitlemeta.VersionTag(track.VersionTag), Format: subtitlemeta.Format(track.Format)}
}

func fileExists(name string) bool {
	info, err := os.Stat(name)
	return err == nil && !info.IsDir()
}

func writeFile(name string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return fmt.Errorf("create subtitle directory: %w", err)
	}
	if err := os.WriteFile(name, body, 0o644); err != nil {
		return fmt.Errorf("write subtitle: %w", err)
	}
	return nil
}
