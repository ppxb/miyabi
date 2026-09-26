package subtitle

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/subtitle"
	"github.com/ppxb/miyabi/internal/pan"
)

const (
	// MaxTracks bounds the subtitles exported per movie, one of each Kind.
	MaxTracks = 3
	// maxDownloads bounds the subtitles downloaded per export: a candidate
	// without a language hint may turn out to repeat an exported kind.
	maxDownloads = 8

	// SourcePan marks tracks indexed from subtitle files beside the 115 videos.
	SourcePan = "115"
	// SourceLocal marks tracks found beside existing local .strm files.
	SourceLocal = "local"
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

// Service exports movie subtitles beside their Emby .strm files.
type Service struct {
	db     *ent.Client
	finder *Finder
}

func NewService(db *ent.Client, finder *Finder) *Service {
	return &Service{db: db, finder: finder}
}

// Export writes a movie's subtitles beside its .strm file and returns how many
// files it wrote. Subtitles stored with the 115 videos come first; online
// subtitles then fill the remaining kinds up to MaxTracks.
func (s *Service) Export(ctx context.Context, reader Reader, movieID int, target Target) (int, error) {
	tracks, err := s.db.Subtitle.Query().Where(subtitle.MovieIDEQ(movieID)).Order(ent.Asc(subtitle.FieldID)).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("list movie subtitles: %w", err)
	}
	exported := make(map[Kind]bool)
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
		switch {
		case track.PickCode != "":
			ok, err := s.exportPanTrack(ctx, reader, track, target, exported)
			if ok {
				written++
			}
			errs = append(errs, err)
		case track.SourceURL != "":
			// The exported file was removed, or predates exports beside the .strm.
			// Drop the record and any stale copy; the online search replaces it.
			if kind := trackKind(track); kind.Format != "" && !exported[kind] {
				_ = os.Remove(target.Path(kind))
			}
			errs = append(errs, s.db.Subtitle.DeleteOne(track).Exec(ctx))
		}
	}
	if !target.HardSubtitled && len(exported) < MaxTracks {
		count, err := s.exportOnline(ctx, movieID, target, exported)
		written += count
		errs = append(errs, err)
	}
	return written, errors.Join(errs...)
}

func (s *Service) exportPanTrack(ctx context.Context, reader Reader, track *ent.Subtitle, target Target, exported map[Kind]bool) (bool, error) {
	format := Format(track.Format)
	if format == "" {
		return false, nil
	}
	raw, err := reader.Read(ctx, track.PickCode, maxSize)
	if err != nil {
		return false, fmt.Errorf("read 115 subtitle %s: %w", track.Name, err)
	}
	body, text, err := Normalize(raw, format)
	if err != nil {
		return false, fmt.Errorf("115 subtitle %s: %w", track.Name, err)
	}
	kind := Kind{Language: DetectLanguage(track.Name, text), Version: VersionTag(track.VersionTag), Format: format}
	if exported[kind] {
		return false, nil
	}
	destination := target.Path(kind)
	if err := writeFile(destination, body); err != nil {
		return false, err
	}
	exported[kind] = true
	return true, s.db.Subtitle.UpdateOne(track).SetLanguage(string(kind.Language)).SetStoragePath(destination).Exec(ctx)
}

type download struct {
	body     []byte
	language Language
	err      error
}

// exportOnline fills the remaining kinds in two passes: first a language or
// cut the movie lacks, then another format of one it has.
func (s *Service) exportOnline(ctx context.Context, movieID int, target Target, exported map[Kind]bool) (int, error) {
	candidates := s.finder.Search(ctx, target.Code, target.Uncensored)
	downloads := make(map[string]download)
	written := make(map[[sha256.Size]byte]bool)
	for _, distinctLanguage := range []bool{true, false} {
		for _, candidate := range candidates {
			if len(exported) >= MaxTracks {
				return len(written), nil
			}
			if candidate.Language != LangUnknown && !wanted(candidate.Kind(), exported, distinctLanguage) {
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
func wanted(kind Kind, exported map[Kind]bool, distinctLanguage bool) bool {
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

// IndexPanTrack records a subtitle file found beside a movie's 115 videos.
// Export copies it into the Emby directory.
func IndexPanTrack(ctx context.Context, tx *ent.Tx, movieID int, file pan.File) error {
	format := Format(path.Ext(file.Name))
	if format == "" {
		return nil
	}
	exists, err := tx.Subtitle.Query().Where(subtitle.MovieIDEQ(movieID), subtitle.FileIDEQ(file.ID)).Exist(ctx)
	if err != nil || exists {
		return err
	}
	return tx.Subtitle.Create().SetMovieID(movieID).SetFileID(file.ID).SetPickCode(file.PickCode).
		SetName(file.Name).SetLanguage(string(DetectLanguage(file.Name, ""))).SetFormat(format).
		SetVersionTag(string(DetectVersion(file.Name))).SetSource(SourcePan).Exec(ctx)
}

func trackKind(track *ent.Subtitle) Kind {
	return Kind{Language: Language(track.Language), Version: VersionTag(track.VersionTag), Format: Format(track.Format)}
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
