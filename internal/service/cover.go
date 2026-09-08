package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/ppxb/miyabi/internal/ent/movie"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
)

func (service *ScrapeService) Cover(ctx context.Context, job TaskJob) error {
	input, err := decodeTaskPayload[coverPayload](job.Payload)
	if err != nil {
		return err
	}
	version, err := service.begin(ctx, input.metadataPayload)
	if err != nil {
		return err
	}
	var artwork mediaimage.Artwork
	switch {
	case input.Origin != nil:
		poster, err := service.originImage(ctx, input.Source, version, input.Origin.Poster)
		if err != nil {
			return err
		}
		fanart, err := service.originImage(ctx, input.Source, version, input.Origin.Fanart)
		if err != nil {
			return err
		}
		artwork, err = service.images.Restore(poster, fanart)
		if err != nil {
			return err
		}
	case input.Artwork != nil:
		artwork = *input.Artwork
	default:
		if input.CoverURL == "" {
			return fmt.Errorf("JavDB 未返回影片封面")
		}
		media, err := service.discover.Media(ctx, input.CoverURL)
		if err != nil {
			return err
		}
		artwork, err = service.images.FromCover(media.Body)
		if err != nil {
			return err
		}
	}
	poster, err := service.images.ReadURL(artwork.Poster)
	if err != nil {
		return fmt.Errorf("read cached poster: %w", err)
	}
	fanart, err := service.images.ReadURL(artwork.Fanart)
	if err != nil {
		return fmt.Errorf("read cached fanart: %w", err)
	}
	// Re-read the directory after scraping. Never upload alongside a video
	// which was deleted or moved while waiting for JavDB or another task.
	directories, err := service.directories(ctx, input.metadataPayload, version)
	if err != nil {
		return err
	}
	for i, directory := range directories {
		if err := service.writeSidecars(ctx, input, version, directory, poster, fanart); err != nil {
			return err
		}
		if err := service.library.database.Task.UpdateOneID(job.ID).SetProgress((i + 1) * 100 / len(directories)).Exec(ctx); err != nil {
			return err
		}
		service.library.tasks.Notify()
	}
	service.library.drive.mu.Lock()
	defer service.library.drive.mu.Unlock()
	if err := service.library.checkScanSource(input.Source, version); err != nil {
		return err
	}
	if err := service.library.database.Movie.UpdateOneID(input.MovieID).
		SetCover(artwork.Thumbnail).SetPoster(artwork.Poster).SetFanarts([]string{artwork.Fanart}).
		SetScrapeStatus(movie.ScrapeStatusDone).Exec(ctx); err != nil {
		return fmt.Errorf("save movie artwork: %w", err)
	}
	service.library.tasks.Notify()
	return nil
}

func (service *ScrapeService) originImage(ctx context.Context, source LibrarySource, version uint64, entry pan.File) ([]byte, error) {
	info, err := service.library.sourceInfo(ctx, source, version, entry.ID)
	if err != nil {
		return nil, fmt.Errorf("find NFO artwork: %w", err)
	}
	return service.library.readSidecar(ctx, source, version, info.File, 32<<20)
}

func (service *ScrapeService) writeSidecars(ctx context.Context, input coverPayload, version uint64, directory movieDirectory, poster, fanart []byte) error {
	nfoName := input.Code + ".nfo"
	// An existing matching NFO is already the source of truth. Preserve its
	// formatting and user edits, as well as its referenced artwork.
	if _, _, found, err := service.directoryNFO(ctx, input.metadataPayload, version, directory); err != nil {
		return err
	} else if found {
		return nil
	}
	posterName, fanartName := "poster.jpg", "fanart.jpg"
	existingPoster, posterExists := sidecarByName(directory.Files, posterName)
	existingFanart, fanartExists := sidecarByName(directory.Files, fanartName)
	if directory.Shared || directory.ID == input.Source.Directory.ID || (posterExists && !strings.EqualFold(existingPoster.SHA1, pan.SHA1(poster))) ||
		(fanartExists && !strings.EqualFold(existingFanart.SHA1, pan.SHA1(fanart))) {
		posterName, fanartName = input.Code+"-poster.jpg", input.Code+"-fanart.jpg"
	}
	doc := input.Document
	doc.Thumbs = []nfo.Thumb{{Aspect: "poster", Path: posterName}}
	doc.Fanart = fanartName
	body, err := nfo.Encode(doc)
	if err != nil {
		return err
	}
	// NFO is the completion marker and is written last. A restarted job can
	// reuse previously uploaded images without creating same-name duplicates.
	for _, item := range []struct {
		name string
		body []byte
	}{
		{posterName, poster}, {fanartName, fanart}, {nfoName, body},
	} {
		if existing, found := sidecarByName(directory.Files, item.name); found {
			if strings.EqualFold(existing.SHA1, pan.SHA1(item.body)) {
				continue
			}
			return fmt.Errorf("媒体目录已存在不同内容的 %s，已保留原文件", item.name)
		}
		if err := service.uploadSidecar(ctx, input.Source, version, directory, item.name, item.body); err != nil {
			return fmt.Errorf("write %s to 115: %w", item.name, err)
		}
	}
	return nil
}

func (service *ScrapeService) uploadSidecar(ctx context.Context, source LibrarySource, version uint64, directory movieDirectory, name string, body []byte) error {
	service.library.drive.mu.Lock()
	defer service.library.drive.mu.Unlock()
	if err := service.library.checkScanSource(source, version); err != nil {
		return err
	}
	_, err := withPanToken(ctx, service.library.drive, func(token string) (struct{}, error) {
		// Confirm the video's current ancestry immediately before writing.
		for _, entry := range directory.Files {
			if !directory.VideoIDs[entry.ID] {
				continue
			}
			info, err := service.library.drive.client.Info(ctx, token, entry.ID)
			if err != nil {
				return struct{}{}, err
			}
			if info.ParentID != directory.ID || !withinSource(info, source) {
				return struct{}{}, fmt.Errorf("视频已移动，请重新扫描")
			}
			return struct{}{}, service.library.drive.client.UploadMetadata(ctx, token, directory.ID, name, body)
		}
		return struct{}{}, fmt.Errorf("没有可写入元数据的视频目录")
	})
	return err
}

func (service *ScrapeService) Artwork(key string) ([]byte, error) {
	return service.images.Read(key)
}
