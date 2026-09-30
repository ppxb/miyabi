package scrape

import (
	"context"
	"fmt"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	subtitlemeta "github.com/ppxb/miyabi/internal/domain/subtitle"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/export"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

// CoverPayload describes the input and intermediate state of a cover creation job.
type CoverPayload struct {
	MetadataPayload
	ScrapeTaskID int                 `json:"scrape_task_id"`
	Document     nfo.Movie           `json:"document"`
	CoverURL     string              `json:"cover_url,omitempty"`
	Artwork      *mediaimage.Artwork `json:"artwork,omitempty"`
	Completed    bool                `json:"completed,omitempty"`
}

// Cover processes the cover download, generation, and export of artwork and NFO sidecars.
func (service *Service) Cover(ctx context.Context, job tasks.Job) error {
	input, err := tasks.DecodePayload[CoverPayload](job.Payload)
	if err != nil {
		return err
	}
	if input.Completed {
		return nil
	}

	subTask, err := service.processCover(ctx, job, input)
	if err != nil {
		return err
	}

	// Subtitle fetching keeps its bounded asynchronous queue.
	if subTask != nil {
		service.subtitleQueue.Enqueue(*subTask)
	}

	return nil
}

func (service *Service) processCover(ctx context.Context, job tasks.Job, input CoverPayload) (*SubtitleTask, error) {
	input.Code = codeid.Normalize(input.Code)
	input.Document.Code = codeid.Normalize(input.Document.Code)
	sess, err := service.begin(ctx, input.MetadataPayload)
	if err != nil {
		return nil, err
	}
	if input.Artwork == nil {
		if input.CoverURL == "" {
			// Older queued jobs may contain only remote NFO artwork.
			if err := service.loadCatalogueCover(ctx, &input); err != nil {
				return nil, err
			}
			if err := sess.Commit(ctx, func(tx *ent.Tx) error {
				return SaveMovieMetadata(ctx, tx, input.MovieID, input.Document)
			}); err != nil {
				return nil, err
			}
		}
		media, err := service.discover.Media(ctx, input.CoverURL)
		if err != nil {
			return nil, err
		}
		if err := service.checkpointArtwork(ctx, job.ID, &input, media.Body); err != nil {
			return nil, err
		}
	}
	// The unfinished task now retains these cache files during remote queries
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
		AccountID:   input.Source.AccountID,
		DirectoryID: input.Source.Directory.ID,
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

	if err := service.exportLocalMedia(ctx, input, videos, poster, fanart); err != nil {
		return nil, err
	}
	input.Completed = true
	encoded, err := tasks.EncodePayload(input)
	if err != nil {
		return nil, err
	}
	if err := sess.Commit(ctx, func(tx *ent.Tx) error {
		if err := tx.Movie.UpdateOneID(input.MovieID).SetCode(input.Code).
			SetCover(artwork.Thumbnail).SetPoster(artwork.Poster).SetFanarts([]string{artwork.Fanart}).
			SetScrapeStatus(movie.ScrapeStatusDone).SetMetadataSnapshot(snapshot).Exec(ctx); err != nil {
			return err
		}
		return tx.Task.UpdateOneID(job.ID).SetPayload(encoded).Exec(ctx)
	}); err != nil {
		return nil, fmt.Errorf("save movie artwork: %w", err)
	}

	subTask := service.subtitleTask(input.MetadataPayload, videos)

	if service.notifier != nil {
		service.notifier.NotifyLibraryChanged()
	}
	return subTask, nil
}

// Protect newly written images until their recovery checkpoint is durable.
// Cache maintenance retains artwork referenced by every unfinished cover task.
func (service *Service) checkpointArtwork(ctx context.Context, taskID int, input *CoverPayload, body []byte) error {
	if err := service.images.LockArtwork(ctx); err != nil {
		return err
	}
	defer service.images.UnlockArtwork()
	artwork, err := service.images.FromCover(body)
	if err != nil {
		return err
	}
	input.Artwork = &artwork
	encoded, err := tasks.EncodePayload(input)
	if err != nil {
		return err
	}
	return service.db.Task.UpdateOneID(taskID).SetPayload(encoded).Exec(ctx)
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
			return domain.E(domain.KindNotFound, "视频文件已删除或无法访问，请重新扫描", err)
		}
		if info.ParentID != directory.ID || !drive.WithinSource(info, sess.Source()) {
			return domain.E(domain.KindConflict, "视频已移动，请重新扫描", nil)
		}
	}
	return nil
}

func (service *Service) exportLocalMedia(ctx context.Context, input CoverPayload, videos []pan.File, poster, fanart []byte) error {
	return service.exportMgr.WithConfig(func(expCfg export.Config) error {
		if err := ExportEmbyMedia(expCfg.EmbyDir, expCfg.PublicURL, expCfg.STRMToken, input.Code, input.Document, videos, poster, fanart); err != nil {
			return err
		}
		if service.mediaNotifier != nil && expCfg.EmbyDir != "" {
			return service.mediaNotifier.NotifyUpdated(ctx, EmbyMovieDir(expCfg.EmbyDir, input.Code))
		}
		return nil
	})
}

// loadCatalogueCover resolves metadata and artwork from the configured catalogue.
func (service *Service) loadCatalogueCover(ctx context.Context, input *CoverPayload) error {
	id := input.JavDBID
	knownID := id != ""
	if id == "" {
		var err error
		id, err = service.discover.ResolveMovieID(ctx, input.Code)
		if err != nil {
			return err
		}
	}
	detail, err := service.discover.CatalogueDetail(ctx, id)
	if err != nil {
		return err
	}
	if !knownID && !codeid.IsEquivalent(detail.Code, input.Code) {
		return domain.E(domain.KindConflict, fmt.Sprintf("JavDB 返回的番号 %s 与媒体文件 %s 不一致", detail.Code, input.Code), nil)
	}
	if detail.Cover == "" {
		return domain.E(domain.KindNotFound, "JavDB 未返回影片封面", nil)
	}
	input.Document = DetailNFO(detail)
	input.Code = codeid.Normalize(input.Document.Code)
	input.Document.Code = input.Code
	input.CoverURL = detail.Cover
	input.JavDBID = id
	return nil
}
