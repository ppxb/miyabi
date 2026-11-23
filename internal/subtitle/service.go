package subtitle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/ppxb/miyabi/internal/atomicfile"
	"github.com/ppxb/miyabi/internal/domain"
	subtitlemeta "github.com/ppxb/miyabi/internal/domain/subtitle"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/subtitle"
	"github.com/ppxb/miyabi/internal/export"
	"github.com/ppxb/miyabi/internal/nfo"
)

// MediaNotifier asks the media server to rescan an exported movie directory.
type MediaNotifier interface {
	NotifyUpdated(ctx context.Context, localPath string) error
}

// Service manages the subtitles a user picks for a movie. Downloads live under
// the application data directory; a movie exported as a single .strm also gets
// a copy beside it so media servers attach the track.
type Service struct {
	db        *ent.Client
	finder    *Finder
	dir       string
	exportDir func() string
	notifier  MediaNotifier
	// files serializes the check-then-write steps of Save, Delete and
	// SyncExport so concurrent calls cannot duplicate a track or resurrect a
	// deleted copy. Downloads stay outside it.
	files sync.Mutex
}

func NewService(db *ent.Client, finder *Finder, dir string, exportDir func() string, notifier MediaNotifier) *Service {
	return &Service{db: db, finder: finder, dir: dir, exportDir: exportDir, notifier: notifier}
}

// Track is a subtitle recorded for a movie. Only downloaded tracks are
// removable; subtitles found beside local media belong to the user.
type Track struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Language  string `json:"language"`
	Format    string `json:"format"`
	Version   string `json:"version"`
	Source    string `json:"source"`
	Removable bool   `json:"removable"`
}

// Offer is an online subtitle the user may preview or save.
type Offer struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Format   string `json:"format"`
	Language string `json:"language"`
	Version  string `json:"version"`
}

func (s *Service) List(ctx context.Context, movieID int) ([]Track, error) {
	records, err := s.db.Subtitle.Query().Where(subtitle.MovieIDEQ(movieID)).Order(ent.Asc(subtitle.FieldID)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list movie subtitles: %w", err)
	}
	tracks := make([]Track, 0, len(records))
	for _, record := range records {
		tracks = append(tracks, trackOf(record))
	}
	return tracks, nil
}

// Search lists online subtitles for the movie without downloading any.
func (s *Service) Search(ctx context.Context, movieID int) ([]Offer, error) {
	record, uncensored, err := s.movie(ctx, movieID)
	if err != nil {
		return nil, err
	}
	candidates := s.finder.Search(ctx, record.Code, uncensored)
	offers := make([]Offer, 0, len(candidates))
	for _, candidate := range candidates {
		offers = append(offers, Offer{
			Provider: candidate.Provider, Name: candidate.Name, URL: candidate.URL, Format: candidate.Format,
			Language: string(candidate.Language), Version: string(candidate.Version),
		})
	}
	return offers, nil
}

// Preview downloads an offer without saving it so the user can judge its text
// and timing against the movie runtime.
func (s *Service) Preview(ctx context.Context, movieID int, offer Offer) (Preview, error) {
	record, _, err := s.movie(ctx, movieID)
	if err != nil {
		return Preview{}, err
	}
	body, language, err := s.download(ctx, offer)
	if err != nil {
		return Preview{}, err
	}
	preview := previewOf(string(bytes.TrimPrefix(body, utf8BOM)), offer.Format)
	preview.Language = string(language)
	if record.Metadata != nil {
		preview.Runtime = record.Metadata.Runtime
	}
	return preview, nil
}

// Save downloads an offer into the data directory and exports a copy. A movie
// keeps one track per language, cut and format, matching Emby's file naming.
func (s *Service) Save(ctx context.Context, movieID int, offer Offer) (Track, error) {
	record, _, err := s.movie(ctx, movieID)
	if err != nil {
		return Track{}, err
	}
	body, language, err := s.download(ctx, offer)
	if err != nil {
		return Track{}, err
	}
	kind := subtitlemeta.Kind{Language: language, Version: versionOf(offer.Version), Format: offer.Format}
	s.files.Lock()
	defer s.files.Unlock()
	existing, err := s.db.Subtitle.Query().Where(subtitle.MovieIDEQ(movieID), subtitle.LanguageEQ(string(kind.Language)),
		subtitle.VersionTagEQ(string(kind.Version)), subtitle.FormatEQ(kind.Format)).First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return Track{}, fmt.Errorf("check movie subtitles: %w", err)
	}
	if existing != nil {
		if existing.SourceURL == "" {
			return Track{}, domain.E(domain.KindConflict, "影片目录中已有相同语言、版本和格式的本地字幕", nil)
		}
		return Track{}, domain.E(domain.KindConflict, "已有相同语言、版本和格式的字幕，请先删除后再下载", nil)
	}
	target := subtitlemeta.Target{Dir: filepath.Join(s.dir, strconv.Itoa(movieID)), Stem: nfo.FileStem(record.Code)}
	destination := target.Path(kind)
	if err := writeFile(destination, body); err != nil {
		return Track{}, err
	}
	provider := strings.TrimSpace(offer.Provider)
	if provider == "" {
		provider = "online"
	}
	saved, err := s.db.Subtitle.Create().SetMovieID(movieID).SetName(filepath.Base(destination)).
		SetLanguage(string(kind.Language)).SetFormat(kind.Format).SetVersionTag(string(kind.Version)).
		SetSource(provider).SetSourceURL(offer.URL).SetStoragePath(destination).Save(ctx)
	if err != nil {
		_ = os.Remove(destination)
		return Track{}, fmt.Errorf("record subtitle: %w", err)
	}
	// The track is saved either way; a failed copy is restored by the next
	// export's SyncExport rather than reported as a failed download.
	if dir := s.exportMovieDir(record.Code); dir != "" {
		if err := copyFile(saved.StoragePath, exportPath(dir, record.Code, kind)); err != nil {
			slog.WarnContext(ctx, "export subtitle copy", "movie_id", movieID, "error", err)
		} else {
			s.notify(ctx, dir)
		}
	}
	return trackOf(saved), nil
}

// Cues reads a movie's track, downloaded or local, as player cues. Local files
// may be GBK or Big5 encoded and are decoded like downloads.
func (s *Service) Cues(ctx context.Context, movieID, trackID int) ([]TimedCue, error) {
	track, err := s.db.Subtitle.Query().Where(subtitle.IDEQ(trackID), subtitle.MovieIDEQ(movieID)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, domain.E(domain.KindNotFound, "字幕不存在", nil)
	}
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(track.StoragePath)
	if err != nil || info.IsDir() {
		return nil, domain.E(domain.KindNotFound, "字幕文件不存在", err)
	}
	if info.Size() > maxSize {
		return nil, domain.E(domain.KindInvalid, "字幕文件过大", nil)
	}
	raw, err := os.ReadFile(track.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("read subtitle: %w", err)
	}
	text, err := DecodeToUTF8(raw)
	if err != nil {
		return nil, domain.E(domain.KindInvalid, "字幕文件无法解码", err)
	}
	return parseCues(text, track.Format), nil
}

// SubtitleText is a saved track decoded to UTF-8, for external players.
type SubtitleText struct {
	Name   string
	Format string
	Text   string
}

// Text reads a track and decodes it to UTF-8 so external players such as mpv
// can load it without caring about the original file encoding.
func (s *Service) Text(ctx context.Context, trackID int) (SubtitleText, error) {
	track, err := s.db.Subtitle.Query().Where(subtitle.IDEQ(trackID)).Only(ctx)
	if ent.IsNotFound(err) {
		return SubtitleText{}, domain.E(domain.KindNotFound, "字幕不存在", nil)
	}
	if err != nil {
		return SubtitleText{}, err
	}
	info, err := os.Stat(track.StoragePath)
	if err != nil || info.IsDir() {
		return SubtitleText{}, domain.E(domain.KindNotFound, "字幕文件不存在", err)
	}
	if info.Size() > maxSize {
		return SubtitleText{}, domain.E(domain.KindInvalid, "字幕文件过大", nil)
	}
	raw, err := os.ReadFile(track.StoragePath)
	if err != nil {
		return SubtitleText{}, fmt.Errorf("read subtitle: %w", err)
	}
	text, err := DecodeToUTF8(raw)
	if err != nil {
		return SubtitleText{}, domain.E(domain.KindInvalid, "字幕文件无法解码", err)
	}
	return SubtitleText{Name: track.Name, Format: track.Format, Text: text}, nil
}

// Delete removes a downloaded track, its file and its exported copy.
func (s *Service) Delete(ctx context.Context, movieID, trackID int) error {
	s.files.Lock()
	defer s.files.Unlock()
	record, _, err := s.movie(ctx, movieID)
	if err != nil {
		return err
	}
	track, err := s.db.Subtitle.Query().Where(subtitle.IDEQ(trackID), subtitle.MovieIDEQ(movieID)).Only(ctx)
	if ent.IsNotFound(err) {
		return domain.E(domain.KindNotFound, "字幕不存在", nil)
	}
	if err != nil {
		return err
	}
	if track.SourceURL == "" {
		return domain.E(domain.KindInvalid, "本地字幕请在其所在目录中管理", nil)
	}
	if err := removeFile(track.StoragePath); err != nil {
		return err
	}
	if dir := s.exportMovieDir(record.Code); dir != "" {
		if exported := exportPath(dir, record.Code, trackKind(track)); exported != track.StoragePath {
			if err := removeFile(exported); err != nil {
				return err
			}
		}
		defer s.notify(ctx, dir)
	}
	return s.db.Subtitle.DeleteOne(track).Exec(ctx)
}

// SyncExport copies a movie's downloaded tracks beside its exported .strm,
// for example after the movie is re-exported under a corrected number.
func (s *Service) SyncExport(ctx context.Context, movieID int) error {
	s.files.Lock()
	defer s.files.Unlock()
	record, err := s.db.Movie.Query().Where(movie.IDEQ(movieID)).Select(movie.FieldID, movie.FieldCode).Only(ctx)
	if err != nil {
		return err
	}
	dir := s.exportMovieDir(record.Code)
	if dir == "" {
		return nil
	}
	tracks, err := s.db.Subtitle.Query().Where(subtitle.MovieIDEQ(movieID), subtitle.SourceURLNEQ("")).All(ctx)
	if err != nil {
		return fmt.Errorf("list movie subtitles: %w", err)
	}
	copied := false
	var errs []error
	for _, track := range tracks {
		destination := exportPath(dir, record.Code, trackKind(track))
		if destination == track.StoragePath || sameContent(track.StoragePath, destination) {
			continue
		}
		if err := copyFile(track.StoragePath, destination); err != nil {
			errs = append(errs, err)
			continue
		}
		copied = true
	}
	if copied {
		s.notify(ctx, dir)
	}
	return errors.Join(errs...)
}

func (s *Service) movie(ctx context.Context, movieID int) (*ent.Movie, bool, error) {
	record, err := s.db.Movie.Query().Where(movie.IDEQ(movieID)).
		WithFiles(func(q *ent.FileQuery) { q.Select(file.FieldMovieID, file.FieldName) }).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, false, domain.E(domain.KindNotFound, "影片不存在", nil)
	}
	if err != nil {
		return nil, false, err
	}
	uncensored := false
	for _, f := range record.Edges.Files {
		uncensored = uncensored || subtitlemeta.IsUncensored(f.Name)
	}
	return record, uncensored, nil
}

func (s *Service) download(ctx context.Context, offer Offer) ([]byte, subtitlemeta.Language, error) {
	format := subtitlemeta.Format(offer.Format)
	if format == "" || format != offer.Format {
		return nil, "", domain.E(domain.KindInvalid, "不支持的字幕格式", nil)
	}
	body, language, err := s.finder.Download(ctx, Candidate{URL: offer.URL, Format: format, Language: languageOf(offer.Language)})
	if err != nil {
		return nil, "", domain.E(domain.KindUpstream, "字幕下载失败或内容无效", err)
	}
	return body, language, nil
}

// exportMovieDir returns the export directory of a movie exported as one
// .strm, or "" when there is none: subtitles fit no part of a multi-part movie.
func (s *Service) exportMovieDir(code string) string {
	if s.exportDir == nil {
		return ""
	}
	base := s.exportDir()
	if base == "" {
		return ""
	}
	dir := export.EmbyMovieDir(base, code)
	if !fileExists(filepath.Join(dir, nfo.FileStem(code)+".strm")) {
		return ""
	}
	return dir
}

func (s *Service) notify(ctx context.Context, dir string) {
	if s.notifier != nil {
		_ = s.notifier.NotifyUpdated(ctx, dir)
	}
}

func exportPath(dir, code string, kind subtitlemeta.Kind) string {
	return subtitlemeta.Target{Dir: dir, Stem: nfo.FileStem(code)}.Path(kind)
}

func trackOf(record *ent.Subtitle) Track {
	return Track{
		ID: record.ID, Name: record.Name, Language: record.Language, Format: record.Format,
		Version: record.VersionTag, Source: record.Source, Removable: record.SourceURL != "",
	}
}

func trackKind(track *ent.Subtitle) subtitlemeta.Kind {
	return subtitlemeta.Kind{Language: subtitlemeta.Language(track.Language), Version: subtitlemeta.VersionTag(track.VersionTag), Format: track.Format}
}

// languageOf accepts only the languages providers label; anything else is
// detected from the downloaded text.
func languageOf(value string) subtitlemeta.Language {
	switch language := subtitlemeta.Language(value); language {
	case subtitlemeta.LangSimplifiedChinese, subtitlemeta.LangTraditionalChinese, subtitlemeta.LangEnglish, subtitlemeta.LangJapanese:
		return language
	default:
		return subtitlemeta.LangUnknown
	}
}

func versionOf(value string) subtitlemeta.VersionTag {
	switch version := subtitlemeta.VersionTag(value); version {
	case subtitlemeta.VersionUncensored, subtitlemeta.VersionExtended, subtitlemeta.VersionLeaked:
		return version
	default:
		return subtitlemeta.VersionStandard
	}
}

func fileExists(name string) bool {
	info, err := os.Stat(name)
	return err == nil && !info.IsDir()
}

func sameContent(a, b string) bool {
	left, err := os.ReadFile(a)
	if err != nil {
		return false
	}
	right, err := os.ReadFile(b)
	return err == nil && bytes.Equal(left, right)
}

func copyFile(source, destination string) error {
	body, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read subtitle: %w", err)
	}
	return writeFile(destination, body)
}

func removeFile(name string) error {
	if name == "" {
		return nil
	}
	if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove subtitle: %w", err)
	}
	return nil
}

func writeFile(name string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return fmt.Errorf("create subtitle directory: %w", err)
	}
	if err := atomicfile.WriteFile(name, body, 0o644); err != nil {
		return fmt.Errorf("write subtitle: %w", err)
	}
	return nil
}
