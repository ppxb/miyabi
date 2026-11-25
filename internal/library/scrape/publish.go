package scrape

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/export"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

func (service *Service) publishMovie(ctx context.Context, sess drive.Session, job tasks.Job, input Payload) error {
	// The unfinished task retains these cache files during remote queries
	// and local export, until the final transaction publishes movie references.
	artwork := *input.Artwork
	poster, err := service.images.ReadURL(artwork.Poster)
	if err != nil {
		return fmt.Errorf("read cached poster: %w", err)
	}
	fanart, err := service.images.ReadURL(artwork.Fanart)
	if err != nil {
		return fmt.Errorf("read cached fanart: %w", err)
	}
	// Verify current video locations before exporting local files.
	directories, err := service.Directories(ctx, sess, input.MetadataPayload)
	if err != nil {
		return err
	}
	snapshot := &domain.MetadataSnapshot{
		Code:          input.Code,
		AccountID:     input.Source.AccountID,
		DirectoryID:   input.Source.Directory.ID,
		PosterVersion: input.PosterVersion,
	}
	var videos []pan.File
	for i, directory := range directories {
		if err := service.verifyVideoPositions(ctx, sess, directory); err != nil {
			return err
		}
		for _, entry := range directory.Files {
			if directory.VideoIDs[entry.ID] {
				videos = append(videos, entry)
			}
		}
		if err := service.db.Task.UpdateOneID(job.ID).SetProgress((i + 1) * 100 / len(directories)).Exec(ctx); err != nil {
			return err
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
		return err
	}
	if err := service.exportMgr.WithConfig(func(cfg export.Config) error {
		return sess.WithSource(ctx, func() error {
			record, err := service.db.Movie.Query().Where(movie.IDEQ(input.MovieID)).
				Select(movie.FieldID, movie.FieldManualCode, movie.FieldMetadataSnapshot).Only(ctx)
			if err != nil {
				return err
			}
			if record.ManualCode != input.ManualCode {
				return domain.E(domain.KindConflict, "影片番号已纠正，请使用新的刮削任务", nil)
			}
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
			if len(current) == 0 {
				return domain.E(domain.KindConflict, "媒体文件索引已变化，请重新扫描", nil)
			}
			if VideoFingerprint(current) != snapshot.Videos {
				// A shared task may overlap a rescan that adds or moves a part.
				// Retry publication from saved artwork using fresh directory listings.
				service.dirMu.Lock()
				clear(service.dirCache)
				service.dirMu.Unlock()
				return &domain.RetryError{Cause: domain.E(domain.KindConflict, "媒体文件索引已更新，等待重新导出", nil)}
			}
			if _, err := export.WriteMedia(cfg, input.Code, input.Document, videos, poster, fanart); err != nil {
				return err
			}
			oldDir, err := service.removePreviousExport(ctx, cfg.EmbyDir, record, input.Code)
			if err != nil {
				return err
			}
			return service.commitPublished(ctx, cfg, job.ID, input, encoded, snapshot, oldDir, true)
		})
	}); err != nil {
		return fmt.Errorf("publish movie: %w", err)
	}

	if service.notifier != nil {
		service.notifier.NotifyLibraryChanged()
	}
	return nil
}

// publishLocal completes a movie found in a local source. Its files are read
// from disk, so there are no remote positions to verify. The Emby export points
// at this server, which streams local file IDs from disk (see /api/strm/play).
func (service *Service) publishLocal(ctx context.Context, job tasks.Job, input Payload) error {
	artwork := *input.Artwork
	poster, err := service.images.ReadURL(artwork.Poster)
	if err != nil {
		return fmt.Errorf("read cached poster: %w", err)
	}
	fanart, err := service.images.ReadURL(artwork.Fanart)
	if err != nil {
		return fmt.Errorf("read cached fanart: %w", err)
	}
	input.Completed = true
	encoded, err := tasks.EncodePayload(input)
	if err != nil {
		return err
	}
	if err := service.exportMgr.WithConfig(func(cfg export.Config) error {
		record, err := service.db.Movie.Query().Where(movie.IDEQ(input.MovieID)).
			Select(movie.FieldID, movie.FieldManualCode, movie.FieldMetadataSnapshot).Only(ctx)
		if err != nil {
			return err
		}
		if record.ManualCode != input.ManualCode {
			return domain.E(domain.KindConflict, "影片番号已纠正，请使用新的刮削任务", nil)
		}
		files, err := service.db.File.Query().Where(movieFiles(input.Source), file.MovieIDEQ(input.MovieID)).All(ctx)
		if err != nil {
			return err
		}
		if len(files) == 0 {
			return domain.E(domain.KindConflict, "媒体文件索引已变化，请重新扫描", nil)
		}
		// The 115 export owns the Emby directory of a movie that also has 115
		// files; writing local .strm files there would remove its .strm files.
		shared, err := service.db.File.Query().
			Where(file.MovieIDEQ(input.MovieID), file.AccountIDNEQ(domain.LocalAccountID)).Exist(ctx)
		if err != nil {
			return err
		}
		exported := !shared && cfg.Validate() == nil
		videos := make([]pan.File, 0, len(files))
		for _, entry := range files {
			// Files inside the export directory are already Emby's; exporting
			// them would overwrite a .strm with a link to itself.
			if exported && withinDir(cfg.EmbyDir, entry.Path) {
				exported = false
			}
			videos = append(videos, pan.File{ID: entry.FileID, Name: entry.Name, Size: entry.Size})
		}
		// Without an export, keep the snapshot describing the existing one so its
		// owner can still find and replace it.
		var snapshot *domain.MetadataSnapshot
		oldDir := ""
		if exported {
			snapshot = &domain.MetadataSnapshot{
				Code:          input.Code,
				AccountID:     input.Source.AccountID,
				DirectoryID:   input.Source.Directory.ID,
				PosterVersion: input.PosterVersion,
				Videos:        VideoFingerprint(videos),
			}
			if _, err := export.WriteMedia(cfg, input.Code, input.Document, videos, poster, fanart); err != nil {
				return err
			}
			if oldDir, err = service.removePreviousExport(ctx, cfg.EmbyDir, record, input.Code); err != nil {
				return err
			}
		}
		return service.commitPublished(ctx, cfg, job.ID, input, encoded, snapshot, oldDir, exported)
	}); err != nil {
		return fmt.Errorf("publish local movie: %w", err)
	}

	if service.notifier != nil {
		service.notifier.NotifyLibraryChanged()
	}
	return nil
}

// withinDir reports whether path lies in dir. Local file paths come from
// symlink-resolved scan roots, so dir is resolved the same way.
func withinDir(dir, path string) bool {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// commitPublished marks the movie done with its published artwork and, when its
// Emby files were written, queues the media server refresh in the same transaction.
func (service *Service) commitPublished(ctx context.Context, cfg export.Config, jobID int, input Payload,
	encoded json.RawMessage, snapshot *domain.MetadataSnapshot, oldDir string, exported bool) error {
	artwork := *input.Artwork
	return ent.WithTx(ctx, service.db, func(tx *ent.Tx) error {
		update := tx.Movie.UpdateOneID(input.MovieID).SetCode(input.Code).SetMetadata(&input.Document).
			SetCover(artwork.Thumbnail).SetPoster(artwork.Poster).SetFanarts([]string{artwork.Fanart}).
			SetScrapeStatus(movie.ScrapeStatusDone)
		if snapshot != nil {
			update.SetMetadataSnapshot(snapshot)
		}
		if err := update.Exec(ctx); err != nil {
			return err
		}
		if err := tx.Task.UpdateOneID(jobID).SetPayload(encoded).Exec(ctx); err != nil {
			return err
		}
		if !exported || service.mediaNotifier == nil || cfg.EmbyDir == "" {
			return nil
		}
		if oldDir != "" {
			if err := service.mediaNotifier.NotifyUpdatedTx(ctx, tx, oldDir); err != nil {
				return err
			}
		}
		return service.mediaNotifier.NotifyUpdatedTx(ctx, tx, export.EmbyMovieDir(cfg.EmbyDir, input.Code))
	})
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
