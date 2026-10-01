package scrape

import (
	"context"
	"fmt"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	subtitlemeta "github.com/ppxb/miyabi/internal/domain/subtitle"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/export"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

func (service *Service) publishMovie(ctx context.Context, sess drive.Session, job tasks.Job, input Payload) (*SubtitleTask, error) {
	// The unfinished task retains these cache files during remote queries
	// and local export, until the final transaction publishes movie references.
	artwork := *input.Artwork
	poster, err := service.images.ReadURL(artwork.Poster)
	if err != nil {
		return nil, fmt.Errorf("read cached poster: %w", err)
	}
	fanart, err := service.images.ReadURL(artwork.Fanart)
	if err != nil {
		return nil, fmt.Errorf("read cached fanart: %w", err)
	}
	// Verify current video locations before exporting local files.
	directories, err := service.Directories(ctx, sess, input.MetadataPayload)
	if err != nil {
		return nil, err
	}
	snapshot := &domain.MetadataSnapshot{
		AccountID:     input.Source.AccountID,
		DirectoryID:   input.Source.Directory.ID,
		PosterVersion: input.PosterVersion,
	}
	var videos []pan.File
	for i, directory := range directories {
		if err := service.verifyVideoPositions(ctx, sess, directory); err != nil {
			return nil, err
		}
		for _, entry := range directory.Files {
			if directory.VideoIDs[entry.ID] {
				videos = append(videos, entry)
			}
		}
		if err := service.db.Task.UpdateOneID(job.ID).SetProgress((i + 1) * 100 / len(directories)).Exec(ctx); err != nil {
			return nil, err
		}
	}
	// Deduplicate and sort all videos across all directories
	seenVideos := make(map[string]bool, len(videos))
	uniqueVideos := make([]pan.File, 0, len(videos))
	for _, v := range videos {
		if !seenVideos[v.ID] {
			seenVideos[v.ID] = true
			uniqueVideos = append(uniqueVideos, v)
		}
	}
	videos = uniqueVideos
	snapshot.Videos = VideoFingerprint(videos)

	input.Completed = true
	encoded, err := tasks.EncodePayload(input)
	if err != nil {
		return nil, err
	}
	if err := service.exportMgr.WithConfig(func(cfg export.Config) error {
		return sess.WithSource(ctx, func() error {
			// A scan may have reconciled the index while artwork was downloading.
			// Check again under the source/export locks before creating any sidecars.
			files, err := service.db.File.Query().Where(database.LibraryFiles(input.Source), file.MovieIDEQ(input.MovieID)).All(ctx)
			if err != nil {
				return err
			}
			current := make([]pan.File, 0, len(files))
			for _, entry := range files {
				current = append(current, pan.File{ID: entry.FileID, ParentID: entry.ParentID, Name: entry.Name, Size: entry.Size, SHA1: entry.Sha1})
			}
			if len(current) == 0 || VideoFingerprint(current) != snapshot.Videos {
				return domain.E(domain.KindConflict, "媒体文件索引已变化，请重新扫描", nil)
			}
			if err := ExportEmbyMedia(cfg.EmbyDir, cfg.PublicURL, cfg.STRMToken, input.Code, input.Document, videos, poster, fanart); err != nil {
				return err
			}
			return ent.WithTx(ctx, service.db, func(tx *ent.Tx) error {
				update := tx.Movie.UpdateOneID(input.MovieID).SetCode(input.Code).SetMetadata(&input.Document).
					SetCover(artwork.Thumbnail).SetPoster(artwork.Poster).SetFanarts([]string{artwork.Fanart}).
					SetScrapeStatus(movie.ScrapeStatusDone).SetMetadataSnapshot(snapshot)
				if id := input.Document.JavDBID(); id != "" {
					update.SetJavdbID(id)
				}
				if err := update.Exec(ctx); err != nil {
					return err
				}
				if err := tx.Task.UpdateOneID(job.ID).SetPayload(encoded).Exec(ctx); err != nil {
					return err
				}
				if service.mediaNotifier != nil && cfg.EmbyDir != "" {
					return service.mediaNotifier.NotifyUpdatedTx(ctx, tx, EmbyMovieDir(cfg.EmbyDir, input.Code))
				}
				return nil
			})
		})
	}); err != nil {
		return nil, fmt.Errorf("publish movie: %w", err)
	}

	subTask := service.subtitleTask(input.MetadataPayload, videos)

	if service.notifier != nil {
		service.notifier.NotifyLibraryChanged()
	}
	return subTask, nil
}

// subtitleTask targets the .strm exported for a movie's video. Multi-part
// movies export one .strm per part, and whole-movie subtitles fit none of them.
func (service *Service) subtitleTask(input MetadataPayload, videos []pan.File) *SubtitleTask {
	if service.subtitles == nil || len(videos) != 1 {
		return nil
	}
	return &SubtitleTask{
		MovieID: input.MovieID,
		Target: subtitlemeta.Target{
			Dir:           EmbyMovieDir(service.exportConfig().EmbyDir, input.Code),
			Stem:          nfo.FileStem(input.Code),
			Code:          input.Code,
			Uncensored:    subtitlemeta.IsUncensored(videos[0].Name),
			HardSubtitled: subtitlemeta.HasHardSubtitle(videos[0].Name),
		},
	}
}

func (service *Service) verifyVideoPositions(ctx context.Context, sess drive.Session, directory MovieDirectory) error {
	for videoID := range directory.VideoIDs {
		info, err := sess.Info(ctx, videoID)
		if err != nil {
			return fmt.Errorf("确认视频文件位置: %w", err)
		}
		if info.ParentID != directory.ID || !drive.WithinSource(info, sess.Source()) {
			return domain.E(domain.KindConflict, "视频已移动，请重新扫描", nil)
		}
	}
	return nil
}
