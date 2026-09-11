package service

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/ent"
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
	if input.Snapshot != nil {
		return nil
	}
	if err := service.artwork.Lock(ctx); err != nil {
		return err
	}
	defer service.artwork.Unlock()
	input.Code = codeid.Normalize(input.Code)
	input.Document.Code = codeid.Normalize(input.Document.Code)
	version, err := service.begin(ctx, input.metadataPayload)
	if err != nil {
		return err
	}
	var artwork mediaimage.Artwork
	switch {
	case input.Artwork != nil:
		artwork = *input.Artwork
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
	input.Artwork = &artwork
	encoded, err := encodeTaskPayload(input)
	if err != nil {
		return err
	}
	if err := service.library.database.Task.UpdateOneID(job.ID).SetPayload(encoded).Exec(ctx); err != nil {
		return err
	}
	// Re-read the directory after scraping. Never upload alongside a video
	// which was deleted or moved while waiting for JavDB or another task.
	directories, err := service.directories(ctx, input.metadataPayload, version)
	if err != nil {
		return err
	}
	snapshot := &metadataSnapshot{}
	var videos []pan.File
	for i, directory := range directories {
		state, err := service.writeSidecars(ctx, input, version, directory, poster, fanart)
		if err != nil {
			return err
		}
		snapshot.Directories = append(snapshot.Directories, state)
		for _, entry := range directory.Files {
			if directory.VideoIDs[entry.ID] {
				videos = append(videos, entry)
			}
		}
		if err := service.library.database.Task.UpdateOneID(job.ID).SetProgress((i + 1) * 100 / len(directories)).Exec(ctx); err != nil {
			return err
		}
	}
	snapshot.Videos = videoFingerprint(videos)
	input.Snapshot = snapshot
	encoded, err = encodeTaskPayload(input)
	if err != nil {
		return err
	}
	if err := service.library.drive.commit.Lock(ctx); err != nil {
		return err
	}
	defer service.library.drive.commit.Unlock()
	if err := service.library.checkScanSource(input.Source, version); err != nil {
		return err
	}
	if err := ent.WithTx(ctx, service.library.database, func(tx *ent.Tx) error {
		if err := tx.Movie.UpdateOneID(input.MovieID).SetCode(input.Code).
			SetCover(artwork.Thumbnail).SetPoster(artwork.Poster).SetFanarts([]string{artwork.Fanart}).
			SetScrapeStatus(movie.ScrapeStatusDone).Exec(ctx); err != nil {
			return err
		}
		return tx.Task.UpdateOneID(job.ID).SetPayload(encoded).Exec(ctx)
	}); err != nil {
		return fmt.Errorf("save movie artwork: %w", err)
	}
	service.library.tasks.NotifyLibraryChanged()
	return nil
}

func (service *ScrapeService) originImage(ctx context.Context, source LibrarySource, version uint64, entry pan.File) ([]byte, error) {
	info, err := service.library.sourceInfo(ctx, source, version, entry.ID)
	if err != nil {
		return nil, fmt.Errorf("find NFO artwork: %w", err)
	}
	return service.library.readSidecar(ctx, source, version, info.File, 32<<20)
}

func (service *ScrapeService) writeSidecars(ctx context.Context, input coverPayload, version uint64, directory movieDirectory, poster, fanart []byte) (metadataDirectorySnapshot, error) {
	var snapshot metadataDirectorySnapshot
	stem := nfo.FileStem(input.Code)
	nfoName := stem + ".nfo"
	// An existing matching NFO is already the source of truth. Preserve its
	// formatting and user edits, as well as its referenced artwork.
	if doc, origin, found, err := service.directoryNFO(ctx, input.metadataPayload, version, directory); err != nil {
		return snapshot, err
	} else if found {
		if err := verifyCoverOrigin(input, directory.ID, doc, *origin, poster, fanart); err != nil {
			return snapshot, err
		}
		return directorySnapshot(directory.ID, origin.NFO, origin.Poster, origin.Fanart), nil
	}
	posterName, fanartName := "poster.jpg", "fanart.jpg"
	existingPoster, posterExists := sidecarByName(directory.Files, posterName)
	existingFanart, fanartExists := sidecarByName(directory.Files, fanartName)
	if directory.Shared || directory.ID == input.Source.Directory.ID || (posterExists && !strings.EqualFold(existingPoster.SHA1, pan.SHA1(poster))) ||
		(fanartExists && !strings.EqualFold(existingFanart.SHA1, pan.SHA1(fanart))) {
		posterName, fanartName = stem+"-poster.jpg", stem+"-fanart.jpg"
	}
	doc := input.Document
	doc.Thumbs = []nfo.Thumb{{Aspect: "poster", Path: posterName}}
	doc.Fanart = fanartName
	body, err := nfo.Encode(doc)
	if err != nil {
		return snapshot, err
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
			return snapshot, fmt.Errorf("媒体目录已存在不同内容的 %s，已保留原文件", item.name)
		}
		if err := service.uploadSidecar(ctx, input.Source, version, directory, item.name, item.body); err != nil {
			return snapshot, fmt.Errorf("write %s to 115: %w", item.name, err)
		}
	}
	return directorySnapshot(directory.ID, pan.File{Name: nfoName, SHA1: pan.SHA1(body)},
		pan.File{Name: posterName, SHA1: pan.SHA1(poster)}, pan.File{Name: fanartName, SHA1: pan.SHA1(fanart)}), nil
}

// A retry may find sidecars written by its previous attempt. Accept those,
// but never mark a newly edited metadata source as already synchronized.
func verifyCoverOrigin(input coverPayload, directoryID string, current nfo.Movie, origin artworkOrigin, poster, fanart []byte) error {
	expected := input.Document
	posterSHA, fanartSHA := pan.SHA1(poster), pan.SHA1(fanart)
	// directories() orders parent IDs as text; later NFOs keep their own edits.
	if input.Origin != nil && directoryID > input.Origin.Poster.ParentID {
		return nil
	}
	if input.Origin != nil && directoryID == input.Origin.Poster.ParentID {
		posterSHA, fanartSHA = input.Origin.Poster.SHA1, input.Origin.Fanart.SHA1
	} else {
		expected.Thumbs = []nfo.Thumb{{Aspect: "poster", Path: origin.Poster.Name}}
		expected.Fanart = origin.Fanart.Name
	}
	before, err := nfo.Encode(expected)
	if err != nil {
		return err
	}
	after, err := nfo.Encode(current)
	if err != nil {
		return err
	}
	if !bytes.Equal(before, after) || !strings.EqualFold(posterSHA, origin.Poster.SHA1) || !strings.EqualFold(fanartSHA, origin.Fanart.SHA1) {
		return fmt.Errorf("NFO 或图片在处理期间发生变化，请重新扫描")
	}
	return nil
}

func (service *ScrapeService) uploadSidecar(ctx context.Context, source LibrarySource, version uint64, directory movieDirectory, name string, body []byte) error {
	state, err := service.library.drive.sourceState(source, version)
	if err != nil {
		return err
	}
	_, err = withPanSourceToken(ctx, service.library.drive, state, func(token string) (struct{}, error) {
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
			if err := service.library.checkScanSource(source, version); err != nil {
				return struct{}{}, err
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
