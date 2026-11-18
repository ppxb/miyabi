package playback

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

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
	record, err := service.database.File.Query().Where(file.FileIDEQ(fileID)).
		Select(file.FieldAccountID, file.FieldRootID).Only(ctx)
	if err != nil {
		return Playback{}, fmt.Errorf("read indexed video: %w", err)
	}
	source, err := service.sourceFor(ctx, record.AccountID, record.RootID)
	if err != nil {
		return Playback{}, err
	}
	sess, err := service.drive.OpenSource(ctx, source)
	if err != nil {
		return Playback{}, err
	}
	if _, err := service.database.File.Query().Where(database.LibraryFiles(source), file.FileIDEQ(fileID)).Only(ctx); err != nil {
		return Playback{}, fmt.Errorf("read indexed video: %w", err)
	}
	info, err := sess.Info(ctx, fileID)
	if err != nil {
		return Playback{}, fmt.Errorf("read 115 video: %w", err)
	}
	if info.IsDirectory || !domain.IsVideo(info.Name) || !drive.WithinSource(info, source) {
		return Playback{}, domain.E(domain.KindNotFound, "视频已不在当前媒体目录中，请重新扫描", fs.ErrNotExist)
	}
	if info.PickCode == "" {
		return Playback{}, fmt.Errorf("115 returned no pick code for video")
	}
	// Prefer the direct download URL: it starts immediately, seeks natively and
	// avoids waiting for a transcode that may still be generating.
	downloadURL, err := sess.DownloadURL(ctx, info.PickCode, userAgent)
	if err == nil {
		return service.createSession(source, []mediaSpec{{address: downloadURL, mediaType: "video/mp4", label: "原画"}})
	}
	if !errors.Is(err, pan.ErrDownloadUnavailable) {
		return Playback{}, fmt.Errorf("get 115 download URL: %w", err)
	}
	sources, err := sess.PlayURL(ctx, info.PickCode, userAgent)
	if err != nil {
		if errors.Is(err, pan.ErrTranscodeUnavailable) {
			return Playback{}, drive.ErrTranscodeUnavailable
		}
		return Playback{}, fmt.Errorf("get 115 playback URL: %w", err)
	}
	specs := make([]mediaSpec, 0, len(sources))
	for _, item := range sources {
		label := fmt.Sprintf("%dp", item.Height)
		if item.Definition == 100 {
			label = "原画 · " + label
		}
		specs = append(specs, mediaSpec{address: item.URL, mediaType: "application/x-mpegurl", label: label, playlist: true})
	}
	return service.createSession(source, specs)
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
	if primary := service.drive.Source(); primary != nil && primary.AccountID == accountID && primary.Directory.ID == rootID {
		return *primary, nil
	}
	source, ok := service.drive.Mount(ctx, accountID)
	if !ok || source.Directory.ID != rootID {
		return domain.LibrarySource{}, domain.E(domain.KindNotFound, "当前挂载目录中未找到该影片的媒体文件", nil)
	}
	return source, nil
}
