package scan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/subtitle"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/nfo"
	subpkg "github.com/ppxb/miyabi/internal/subtitle"
)

var playURLRegex = regexp.MustCompile(`/api/strm/play/([a-zA-Z0-9_\-]+)`)

// LocalScanResult summarizes the outcome of a local directory scan.
type LocalScanResult struct {
	FilesScanned int `json:"files_scanned"`
	MediaFiles   int `json:"media_files"`
	MoviesAdded  int `json:"movies_added"`
	NFORead      int `json:"nfo_read"`
}

// LocalScanner scans a local directory structure containing .strm and video files.
type LocalScanner struct {
	db       *ent.Client
	images   *mediaimage.Cache
	notifier MediaNotifier
}

// NewLocalScanner creates a new LocalScanner.
func NewLocalScanner(db *ent.Client, images *mediaimage.Cache) *LocalScanner {
	return &LocalScanner{
		db:     db,
		images: images,
	}
}

// SetMediaNotifier sets the notification handler for discovered media folders.
func (s *LocalScanner) SetMediaNotifier(notifier MediaNotifier) {
	s.notifier = notifier
}

type dirGroup struct {
	mediaFiles []fs.FileInfo
	nfoFiles   []string
	imageFiles []string
	subFiles   []string
}

// Scan walks rootDir and imports any found media (.strm, .mp4, etc.) into the SQLite database.
func (s *LocalScanner) Scan(ctx context.Context, rootDir string) (*LocalScanResult, error) {
	stat, err := os.Stat(rootDir)
	if err != nil {
		return nil, fmt.Errorf("stat local directory %s: %w", rootDir, err)
	}
	if !stat.IsDir() {
		return nil, fmt.Errorf("local path %s is not a directory", rootDir)
	}

	result := &LocalScanResult{}
	dirs := make(map[string]*dirGroup)

	err = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			return nil
		}
		result.FilesScanned++
		dir := filepath.Dir(path)
		group := dirs[dir]
		if group == nil {
			group = &dirGroup{}
			dirs[dir] = group
		}
		name := d.Name()
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}

		if domain.IsMedia(name) {
			group.mediaFiles = append(group.mediaFiles, info)
		} else if strings.EqualFold(filepath.Ext(name), ".nfo") {
			group.nfoFiles = append(group.nfoFiles, path)
		} else if isImageFile(name) {
			group.imageFiles = append(group.imageFiles, path)
		} else if domain.IsSubtitle(name) {
			group.subFiles = append(group.subFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk local directory %s: %w", rootDir, err)
	}

	for dir, group := range dirs {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if len(group.mediaFiles) == 0 {
			continue
		}
		for _, media := range group.mediaFiles {
			mediaPath := filepath.Join(dir, media.Name())
			stem := strings.TrimSuffix(media.Name(), filepath.Ext(media.Name()))

			code, ok := codeid.Parse(media.Name())
			if !ok || code == "" {
				code, ok = codeid.Parse(filepath.Base(dir))
			}
			if !ok || code == "" {
				continue
			}
			result.MediaFiles++

			nfoPath := findMatchingNFO(dir, stem, code, group.nfoFiles)
			posterPath, fanartPath := findMatchingArtwork(dir, stem, group.imageFiles)
			subPaths := findMatchingSubtitles(stem, group.subFiles)

			if err := s.ingestMedia(ctx, rootDir, mediaPath, media.Name(), media.Size(), code, nfoPath, posterPath, fanartPath, subPaths, result); err != nil {
				return nil, fmt.Errorf("ingest %s: %w", mediaPath, err)
			}
		}
	}

	return result, nil
}

func (s *LocalScanner) ingestMedia(
	ctx context.Context,
	rootDir, mediaPath, mediaName string,
	mediaSize int64,
	code string,
	nfoPath, posterPath, fanartPath string,
	subPaths []string,
	result *LocalScanResult,
) error {
	return ent.WithTx(ctx, s.db, func(tx *ent.Tx) error {
		matched, err := MatchMovies(ctx, tx, []string{code})
		if err != nil {
			return err
		}
		movieID, found := matched[code]
		if !found {
			created, err := tx.Movie.Create().SetCode(code).SetScrapeStatus(movie.ScrapeStatusPending).Save(ctx)
			if err != nil {
				return err
			}
			movieID = created.ID
			result.MoviesAdded++
		}

		if nfoPath != "" {
			nfoBytes, readErr := os.ReadFile(nfoPath)
			if readErr == nil {
				doc, decodeErr := nfo.Decode(nfoBytes)
				if decodeErr == nil {
					result.NFORead++
					if err := scrape.SaveMovieMetadata(ctx, tx, movieID, doc); err == nil {
						_ = tx.Movie.UpdateOneID(movieID).SetScrapeStatus(movie.ScrapeStatusDone).Exec(ctx)
					}
				}
			}
		}

		if s.images != nil && (posterPath != "" || fanartPath != "") {
			var posterBytes, fanartBytes []byte
			if posterPath != "" {
				posterBytes, _ = os.ReadFile(posterPath)
			}
			if fanartPath != "" {
				fanartBytes, _ = os.ReadFile(fanartPath)
			}
			if len(posterBytes) > 0 {
				var artwork mediaimage.Artwork
				var artErr error
				if len(fanartBytes) > 0 {
					artwork, artErr = s.images.Restore(posterBytes, fanartBytes)
				} else {
					artwork, artErr = s.images.FromCover(posterBytes)
				}
				if artErr == nil {
					update := tx.Movie.UpdateOneID(movieID)
					if artwork.Poster != "" {
						update.SetPoster(artwork.Poster)
					}
					if artwork.Thumbnail != "" {
						update.SetCover(artwork.Thumbnail)
					}
					if artwork.Fanart != "" {
						update.SetFanarts([]string{artwork.Fanart})
					}
					_ = update.Exec(ctx)
				}
			}
		}

		fileID := ""
		if domain.IsSTRM(mediaName) {
			content, _ := os.ReadFile(mediaPath)
			if match := playURLRegex.FindSubmatch(content); len(match) > 1 {
				fileID = string(match[1])
			}
		}
		rel, _ := filepath.Rel(rootDir, mediaPath)
		if fileID == "" {
			hash := sha256.Sum256([]byte(rel))
			fileID = "local-" + hex.EncodeToString(hash[:16])
		}

		existingFile, err := tx.File.Query().Where(file.FileIDEQ(fileID)).First(ctx)
		if ent.IsNotFound(err) {
			_, err = tx.File.Create().
				SetFileID(fileID).
				SetName(mediaName).
				SetSize(mediaSize).
				SetAccountID("local").
				SetRootID("local").
				SetPath(rel).
				SetMovieID(movieID).
				Save(ctx)
			if err != nil {
				return err
			}
		} else if err == nil && (existingFile.MovieID == nil || *existingFile.MovieID != movieID) {
			_ = tx.File.UpdateOneID(existingFile.ID).SetMovieID(movieID).Exec(ctx)
		}

		for _, subPath := range subPaths {
			if err := indexLocalSubtitle(ctx, tx, movieID, subPath); err != nil {
				return err
			}
		}
		if s.notifier != nil {
			s.notifier.NotifyUpdated(filepath.Dir(mediaPath))
		}
		return nil
	})
}

func isImageFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp"
}

func findMatchingNFO(dir, stem, code string, nfoFiles []string) string {
	candidates := []string{
		filepath.Join(dir, stem+".nfo"),
		filepath.Join(dir, code+".nfo"),
		filepath.Join(dir, "movie.nfo"),
	}
	for _, cand := range candidates {
		for _, f := range nfoFiles {
			if strings.EqualFold(f, cand) {
				return f
			}
		}
	}
	if len(nfoFiles) == 1 {
		return nfoFiles[0]
	}
	return ""
}

func findMatchingArtwork(dir, stem string, imageFiles []string) (posterPath, fanartPath string) {
	posterNames := []string{
		"poster.jpg", "cover.jpg",
		stem + "-poster.jpg", stem + "-cover.jpg", stem + ".jpg",
		"poster.png", "cover.png",
	}
	fanartNames := []string{
		"fanart.jpg", stem + "-fanart.jpg",
		"fanart.png", stem + "-fanart.png",
	}

	for _, name := range posterNames {
		target := filepath.Join(dir, name)
		for _, f := range imageFiles {
			if strings.EqualFold(f, target) {
				posterPath = f
				break
			}
		}
		if posterPath != "" {
			break
		}
	}

	for _, name := range fanartNames {
		target := filepath.Join(dir, name)
		for _, f := range imageFiles {
			if strings.EqualFold(f, target) {
				fanartPath = f
				break
			}
		}
		if fanartPath != "" {
			break
		}
	}
	return posterPath, fanartPath
}

// findMatchingSubtitles returns the subtitles named after a media file, such
// as IPX-123.zh-CN.srt for IPX-123.strm. A lone subtitle belongs to the lone
// media file in its directory.
func findMatchingSubtitles(stem string, subFiles []string) []string {
	var matches []string
	for _, f := range subFiles {
		if strings.HasPrefix(strings.ToLower(filepath.Base(f)), strings.ToLower(stem)+".") {
			matches = append(matches, f)
		}
	}
	if len(matches) == 0 && len(subFiles) == 1 {
		return subFiles
	}
	return matches
}

// indexLocalSubtitle records a subtitle already beside a local .strm so its
// kind is not exported again.
func indexLocalSubtitle(ctx context.Context, tx *ent.Tx, movieID int, subPath string) error {
	name := filepath.Base(subPath)
	format := subpkg.Format(filepath.Ext(name))
	if format == "" {
		return nil
	}
	exists, err := tx.Subtitle.Query().Where(subtitle.MovieIDEQ(movieID), subtitle.StoragePathEQ(subPath)).Exist(ctx)
	if err != nil || exists {
		return err
	}
	return tx.Subtitle.Create().SetMovieID(movieID).SetName(name).SetFormat(format).
		SetLanguage(string(subpkg.DetectLanguage(name, ""))).SetVersionTag(string(subpkg.DetectVersion(name))).
		SetSource(subpkg.SourceLocal).SetStoragePath(subPath).Exec(ctx)
}
