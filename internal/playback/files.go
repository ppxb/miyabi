package playback

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/pan"
)

// Files lists the playable videos of one library movie.
func (service *Service) Files(ctx context.Context, movieID int) (PlayFiles, error) {
	source, err := service.movieSource(ctx, movieID)
	if err != nil {
		return PlayFiles{}, err
	}
	scope := database.LibraryFiles(source)
	record, err := service.database.Movie.Query().
		Where(movie.IDEQ(movieID), movie.HasFilesWith(scope)).
		WithFiles(func(query *ent.FileQuery) {
			query.Where(scope).Order(ent.Desc(file.FieldSize), ent.Asc(file.FieldPath), ent.Asc(file.FieldID))
		}).Only(ctx)
	if err != nil {
		return PlayFiles{}, fmt.Errorf("read playable movie: %w", err)
	}
	result := PlayFiles{
		Code: record.Code, Title: record.Title, Source: source,
		Files: make([]domain.LibraryFile, 0, len(record.Edges.Files)),
	}
	for _, entry := range record.Edges.Files {
		result.Files = append(result.Files, domain.LibraryFile{ID: entry.FileID, Name: entry.Name, Path: entry.Path, Size: entry.Size})
	}
	return result, nil
}

// Start creates a playback session for one indexed video file.
func (service *Service) Start(ctx context.Context, fileID, userAgent string) (Playback, error) {
	opened, err := service.openFile(ctx, fileID)
	if err != nil {
		return Playback{}, err
	}
	// Prefer the direct download URL: it starts immediately, seeks natively and
	// avoids waiting for a transcode that may still be generating.
	specs, err := service.resolveSpecs(ctx, opened.sess, opened.source.AccountID, opened.pickCode, userAgent)
	if err != nil {
		return Playback{}, err
	}
	result, err := service.createSession(opened.source, specs)
	if err != nil {
		return Playback{}, err
	}
	result.FileID = fileID
	return result, nil
}

// StartHLS opens a transcode session with the available video renditions.
func (service *Service) StartHLS(ctx context.Context, fileID, userAgent string) (Playback, error) {
	opened, err := service.openFile(ctx, fileID)
	if err != nil {
		return Playback{}, err
	}
	sources, err := opened.sess.PlayURL(ctx, opened.pickCode, userAgent)
	if err != nil {
		if errors.Is(err, pan.ErrTranscodeUnavailable) {
			return Playback{}, drive.ErrTranscodeUnavailable
		}
		return Playback{}, fmt.Errorf("get 115 playback URL: %w", err)
	}
	result, err := service.createSession(opened.source, hlsSpecs(sources))
	if err != nil {
		return Playback{}, err
	}
	result.FileID = fileID
	return result, nil
}

type openedFile struct {
	source   domain.LibrarySource
	sess     drive.Session
	pickCode string
	name     string
}

func (service *Service) openFile(ctx context.Context, fileID string) (openedFile, error) {
	record, err := service.database.File.Query().Where(file.FileIDEQ(fileID)).
		Select(file.FieldAccountID, file.FieldRootID, file.FieldPickCode, file.FieldName).Only(ctx)
	if err != nil {
		return openedFile{}, fmt.Errorf("read indexed video: %w", err)
	}
	return service.openFileRecord(ctx, record)
}

func (service *Service) openFileRecord(ctx context.Context, record *ent.File) (openedFile, error) {
	fileID := record.FileID
	source, err := service.sourceFor(ctx, record.AccountID, record.RootID)
	if err != nil {
		return openedFile{}, err
	}
	sess, err := service.drive.OpenSource(ctx, source)
	if err != nil {
		return openedFile{}, err
	}
	pickCode, name := record.PickCode, record.Name
	// Only ask 115 for file info when the index predates pick-code storage.
	if pickCode == "" {
		info, err := sess.Info(ctx, fileID)
		if err != nil {
			return openedFile{}, fmt.Errorf("read 115 video: %w", err)
		}
		if info.IsDirectory || !domain.IsVideo(info.Name) || !drive.WithinSource(info, source) {
			return openedFile{}, domain.E(domain.KindNotFound, "视频已不在当前媒体目录中，请重新扫描", fs.ErrNotExist)
		}
		pickCode, name = info.PickCode, info.Name
	}
	if !domain.IsVideo(name) {
		return openedFile{}, domain.E(domain.KindNotFound, "视频已不在当前媒体目录中，请重新扫描", fs.ErrNotExist)
	}
	if pickCode == "" {
		return openedFile{}, fmt.Errorf("115 returned no pick code for video")
	}
	return openedFile{source: source, sess: sess, pickCode: pickCode, name: name}, nil
}

// StartMovie starts playback for a movie's primary video in one call.
func (service *Service) StartMovie(ctx context.Context, movieID int, userAgent string) (Playback, error) {
	var (
		records   []*ent.File
		resume    *domain.WatchResume
		fileErr   error
		resumeErr error
	)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		records, fileErr = service.database.File.Query().
			Where(file.MovieIDEQ(movieID)).
			Select(file.FieldFileID, file.FieldSize, file.FieldAccountID, file.FieldRootID, file.FieldPickCode, file.FieldName, file.FieldPath).
			Order(ent.Desc(file.FieldSize), ent.Asc(file.FieldPath), ent.Asc(file.FieldID)).All(ctx)
	}()
	go func() {
		defer wg.Done()
		resume, resumeErr = service.Resume(ctx, movieID)
	}()
	wg.Wait()
	if fileErr != nil {
		return Playback{}, fmt.Errorf("read indexed videos: %w", fileErr)
	}
	if resumeErr != nil {
		return Playback{}, resumeErr
	}
	target, resume := service.selectPlayableFile(ctx, records, resume)
	if target == nil {
		return Playback{}, domain.E(domain.KindNotFound, "当前挂载目录中未找到可播放的文件", nil)
	}
	if target.AccountID == domain.LocalAccountID {
		return service.startLocalPlayback(ctx, target, resume)
	}
	opened, err := service.openFileRecord(ctx, target)
	if err != nil {
		return Playback{}, err
	}
	specs, err := service.resolveSpecs(ctx, opened.sess, opened.source.AccountID, opened.pickCode, userAgent)
	if err != nil {
		return Playback{}, err
	}
	result, err := service.createSession(opened.source, specs)
	if err != nil {
		return Playback{}, err
	}
	result.FileID = target.FileID
	result.Resume = resume
	return result, nil
}

// selectPlayableFile picks the file StartMovie/Mpv should play: the resumed file
// when present, otherwise the largest. A local pick that can no longer be located
// falls back to a playable cloud file so a removed local root never blocks play.
func (service *Service) selectPlayableFile(ctx context.Context, records []*ent.File, resume *domain.WatchResume) (*ent.File, *domain.WatchResume) {
	if len(records) == 0 {
		return nil, nil
	}
	target := records[0]
	if resume != nil {
		matched := false
		for _, item := range records {
			if item.FileID == resume.FileID {
				target, matched = item, true
				break
			}
		}
		if !matched {
			resume = nil
		}
	}
	if target.AccountID == domain.LocalAccountID {
		if _, err := service.LocateLocalFile(ctx, target.FileID); err != nil {
			var fallback *ent.File
			for _, item := range records {
				if item.AccountID != domain.LocalAccountID {
					fallback = item
					break
				}
			}
			if fallback == nil {
				return nil, nil
			}
			target, resume = fallback, nil
		}
	}
	return target, resume
}

// PrimaryFile reports the file MPV should open for a movie, and whether it is a
// local file (served from disk) or a cloud file (served through the 115 relay).
type PrimaryFile struct {
	FileID string `json:"file_id"`
	Name   string `json:"name"`
	Local  bool   `json:"local"`
}

func (service *Service) PrimaryFile(ctx context.Context, movieID int) (PrimaryFile, error) {
	records, err := service.database.File.Query().
		Where(file.MovieIDEQ(movieID)).
		Select(file.FieldFileID, file.FieldName, file.FieldSize, file.FieldAccountID, file.FieldPath).
		Order(ent.Desc(file.FieldSize), ent.Asc(file.FieldPath), ent.Asc(file.FieldID)).All(ctx)
	if err != nil {
		return PrimaryFile{}, fmt.Errorf("read indexed videos: %w", err)
	}
	target, _ := service.selectPlayableFile(ctx, records, nil)
	if target == nil {
		return PrimaryFile{}, domain.E(domain.KindNotFound, "当前挂载目录中未找到可播放的文件", nil)
	}
	return PrimaryFile{FileID: target.FileID, Name: target.Name, Local: target.AccountID == domain.LocalAccountID}, nil
}

// playSourceSession is the subset of a drive session needed to resolve media.
type playSourceSession interface {
	AuthorizationVersion() uint64
	DownloadURL(ctx context.Context, pickCode, userAgent string) (string, error)
	PlayURL(ctx context.Context, pickCode, userAgent string) ([]pan.PlaySource, error)
}

// mediaSpecs prefers the direct download URL and falls back to transcoded HLS.
func mediaSpecs(ctx context.Context, sess playSourceSession, pickCode, userAgent string) ([]mediaSpec, error) {
	downloadURL, err := sess.DownloadURL(ctx, pickCode, userAgent)
	if err == nil {
		return []mediaSpec{{address: downloadURL, mediaType: "video/mp4", label: "原画"}}, nil
	}
	if !errors.Is(err, pan.ErrDownloadUnavailable) {
		return nil, fmt.Errorf("get 115 download URL: %w", err)
	}
	sources, err := sess.PlayURL(ctx, pickCode, userAgent)
	if err != nil {
		if errors.Is(err, pan.ErrTranscodeUnavailable) {
			return nil, drive.ErrTranscodeUnavailable
		}
		return nil, fmt.Errorf("get 115 playback URL: %w", err)
	}
	return hlsSpecs(sources), nil
}

// resolveSpecs prefers a recently resolved 115 direct URL so reopening the same
// file starts without another round trip. Only direct mp4 URLs are cached;
// transcoded HLS is always re-resolved. 115 binds a direct URL to the user agent
// that requested it, and a new login re-authorizes the account, so both are part
// of the key (as in the STRM relay).
func (service *Service) resolveSpecs(ctx context.Context, sess playSourceSession, accountID, pickCode, userAgent string) ([]mediaSpec, error) {
	key := fmt.Sprintf("%d:%q:%q:%q", sess.AuthorizationVersion(), accountID, pickCode, strings.TrimSpace(userAgent))
	if address, ok := service.cachedDownloadURL(key); ok {
		return []mediaSpec{{address: address, mediaType: "video/mp4", label: "原画"}}, nil
	}
	specs, err := mediaSpecs(ctx, sess, pickCode, userAgent)
	if err != nil {
		return nil, err
	}
	if len(specs) == 1 && specs[0].mediaType == "video/mp4" && !specs[0].playlist && specs[0].address != "" {
		service.storeDownloadURL(key, specs[0].address)
	}
	return specs, nil
}

func hlsSpecs(sources []pan.PlaySource) []mediaSpec {
	specs := make([]mediaSpec, 0, len(sources))
	for _, item := range sources {
		label := fmt.Sprintf("%dp", item.Height)
		if item.Definition == 100 {
			label = "原画 · " + label
		}
		specs = append(specs, mediaSpec{address: item.URL, mediaType: "application/x-mpegurl", label: label, playlist: true})
	}
	return specs
}

func (service *Service) movieSource(ctx context.Context, movieID int) (domain.LibrarySource, error) {
	record, err := service.database.File.Query().
		Where(file.MovieIDEQ(movieID), file.AccountIDNEQ(domain.LocalAccountID)).
		Select(file.FieldAccountID, file.FieldRootID).First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return domain.LibrarySource{}, domain.E(domain.KindNotFound, "当前挂载目录中未找到该影片的媒体文件", nil)
		}
		return domain.LibrarySource{}, err
	}
	return service.sourceFor(ctx, record.AccountID, record.RootID)
}

func (service *Service) sourceFor(ctx context.Context, accountID, rootID string) (domain.LibrarySource, error) {
	source, ok, err := service.drive.Mount(ctx, accountID)
	if err != nil {
		return domain.LibrarySource{}, err
	}
	if !ok || source.Directory.ID != rootID {
		return domain.LibrarySource{}, domain.E(domain.KindNotFound, "当前挂载目录中未找到该影片的媒体文件", nil)
	}
	return source, nil
}

// LocateLocalFile resolves an indexed local file to its absolute path on disk.
// Paths that leave every configured local source directory are refused, and a
// path that exists under more than one root is reported as ambiguous instead of
// silently picking one, since file rows do not record which root they came from.
func (service *Service) LocateLocalFile(ctx context.Context, fileID string) (string, error) {
	record, err := service.database.File.Query().
		Where(file.FileIDEQ(fileID), file.AccountIDEQ(domain.LocalAccountID)).
		Select(file.FieldPath).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", domain.E(domain.KindNotFound, "本地文件不存在，请重新扫描", fs.ErrNotExist)
		}
		return "", err
	}
	if service.localRoots == nil {
		return "", domain.E(domain.KindNotFound, "本地文件不在已配置的本地目录中，请重新扫描", fs.ErrNotExist)
	}
	roots, err := service.localRoots(ctx)
	if err != nil {
		return "", err
	}
	var canonicalRoots []string
	for _, root := range roots {
		if canonical := canonicalPath(root); canonical != "" {
			canonicalRoots = append(canonicalRoots, canonical)
		}
	}
	var candidates []string
	if filepath.IsAbs(record.Path) {
		candidates = []string{record.Path}
	} else {
		for _, root := range canonicalRoots {
			candidates = append(candidates, filepath.Join(root, record.Path))
		}
	}
	var found []string
	for _, candidate := range candidates {
		real := canonicalPath(candidate)
		if real == "" || !withinAny(canonicalRoots, real) {
			continue
		}
		if info, err := os.Stat(real); err == nil && info.Mode().IsRegular() {
			found = append(found, real)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", domain.E(domain.KindNotFound, "本地文件不在已配置的本地目录中，请重新扫描", fs.ErrNotExist)
	default:
		return "", domain.E(domain.KindConflict, "本地文件在多个目录中都存在，无法确定播放哪个，请检查本地源配置", nil)
	}
}

// canonicalPath returns the absolute, symlink-resolved form of a path, or "" on
// error (for example a path that does not exist yet).
func canonicalPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

func withinAny(roots []string, target string) bool {
	for _, root := range roots {
		rel, err := filepath.Rel(root, target)
		if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return true
	}
	return false
}

// startLocalPlayback exposes a local file through this server (see the
// /api/play-local route) instead of the 115 proxy session.
func (service *Service) startLocalPlayback(ctx context.Context, record *ent.File, resume *domain.WatchResume) (Playback, error) {
	if !domain.IsMedia(record.Name) {
		return Playback{}, domain.E(domain.KindNotFound, "视频已不在当前媒体目录中，请重新扫描", fs.ErrNotExist)
	}
	return Playback{
		FileID:  record.FileID,
		Sources: []MediaSource{{Src: "/api/play-local/" + record.FileID, Type: service.localMediaType(ctx, record), Label: "原画"}},
		Source: domain.LibrarySource{
			AccountID: domain.LocalAccountID,
			Directory: domain.LibraryDirectory{ID: domain.LocalAccountID, Name: "本地目录"},
		},
		Resume: resume,
	}, nil
}

func (service *Service) localMediaType(ctx context.Context, record *ent.File) string {
	if strings.EqualFold(filepath.Ext(record.Name), ".m3u8") {
		return "application/x-mpegurl"
	}
	if strings.EqualFold(filepath.Ext(record.Name), ".strm") {
		if path, err := service.LocateLocalFile(ctx, record.FileID); err == nil {
			if body, err := os.ReadFile(path); err == nil && isHLSURL(firstNonBlankLine(string(body))) {
				return "application/x-mpegurl"
			}
		}
	}
	return "video/mp4"
}

func firstNonBlankLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func isHLSURL(target string) bool {
	parsed, err := url.Parse(target)
	if err != nil {
		return false
	}
	ext := strings.ToLower(filepath.Ext(parsed.Path))
	return ext == ".m3u8" || ext == ".m3u"
}
