package scrape

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/subtitle"
	"github.com/ppxb/miyabi/internal/tasks"
)

// CoverPayload describes the input and intermediate state of a cover creation job.
type CoverPayload struct {
	MetadataPayload
	ScrapeTaskID int                 `json:"scrape_task_id"`
	Document     nfo.Movie           `json:"document"`
	CoverURL     string              `json:"cover_url,omitempty"`
	Origin       *ArtworkOrigin      `json:"origin,omitempty"`
	Artwork      *mediaimage.Artwork `json:"artwork,omitempty"`
	Snapshot     *Snapshot           `json:"snapshot,omitempty"`
}

// Cover processes the cover download, generation, and upload of artwork and NFO sidecars.
func (service *Service) Cover(ctx context.Context, job tasks.Job) error {
	input, err := tasks.DecodePayload[CoverPayload](job.Payload)
	if err != nil {
		return err
	}
	if input.Snapshot != nil {
		return nil
	}

	var subTask *SubtitleTask
	err = func() error {
		if err := service.artwork.Lock(ctx); err != nil {
			return err
		}
		defer service.artwork.Unlock()

		var err error
		subTask, err = service.processCover(ctx, job, input)
		return err
	}()
	if err != nil {
		return err
	}

	// Dispatch subtitle fetching asynchronously via bounded queue after releasing the artwork lock
	// to avoid blocking other movies' scrape and artwork pipelines and prevent unbounded goroutines.
	if subTask != nil && service.subtitleQueue != nil {
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
	var artwork mediaimage.Artwork
	switch {
	case input.Artwork != nil:
		artwork = *input.Artwork
	case input.Origin != nil:
		poster, err := service.originImage(ctx, sess, input.Origin.Poster)
		if err != nil {
			return nil, err
		}
		fanart, err := service.originImage(ctx, sess, input.Origin.Fanart)
		if err != nil {
			return nil, err
		}
		artwork, err = service.images.Restore(poster, fanart)
		if err != nil {
			return nil, err
		}
	default:
		if input.CoverURL == "" {
			return nil, domain.E(domain.KindNotFound, "JavDB 未返回影片封面", nil)
		}
		media, err := service.discover.Media(ctx, input.CoverURL)
		if err != nil {
			return nil, err
		}
		artwork, err = service.images.FromCover(media.Body)
		if err != nil {
			return nil, err
		}
	}
	poster, err := service.images.ReadURL(artwork.Poster)
	if err != nil {
		return nil, fmt.Errorf("read cached poster: %w", err)
	}
	fanart, err := service.images.ReadURL(artwork.Fanart)
	if err != nil {
		return nil, fmt.Errorf("read cached fanart: %w", err)
	}
	input.Artwork = &artwork
	encoded, err := tasks.EncodePayload(input)
	if err != nil {
		return nil, err
	}
	if err := service.db.Task.UpdateOneID(job.ID).SetPayload(encoded).Exec(ctx); err != nil {
		return nil, err
	}
	// Re-read the directory after scraping. Never upload alongside a video
	// which was deleted or moved while waiting for JavDB or another task.
	directories, err := service.directories(ctx, sess, input.MetadataPayload)
	if err != nil {
		return nil, err
	}
	snapshot := &Snapshot{
		LocalExport: true,
	}
	var videos []pan.File
	for i, directory := range directories {
		if err := service.verifyVideoPositions(ctx, sess, directory); err != nil {
			return nil, err
		}
		var dirVideos []pan.File
		for _, entry := range directory.Files {
			if directory.VideoIDs[entry.ID] {
				videos = append(videos, entry)
				dirVideos = append(dirVideos, entry)
			}
		}
		state, err := service.writeSidecars(ctx, sess, input, directory, dirVideos, poster, fanart)
		if err != nil {
			return nil, err
		}
		snapshot.Directories = append(snapshot.Directories, state)
		if err := service.db.Task.UpdateOneID(job.ID).SetProgress((i + 1) * 100 / len(directories)).Exec(ctx); err != nil {
			return nil, err
		}
	}
	snapshot.Videos = VideoFingerprint(videos)
	input.Snapshot = snapshot
	encoded, err = tasks.EncodePayload(input)
	if err != nil {
		return nil, err
	}
	if err := sess.Commit(ctx, func(tx *ent.Tx) error {
		if err := tx.Movie.UpdateOneID(input.MovieID).SetCode(input.Code).
			SetCover(artwork.Thumbnail).SetPoster(artwork.Poster).SetFanarts([]string{artwork.Fanart}).
			SetScrapeStatus(movie.ScrapeStatusDone).Exec(ctx); err != nil {
			return err
		}
		return tx.Task.UpdateOneID(job.ID).SetPayload(encoded).Exec(ctx)
	}); err != nil {
		return nil, fmt.Errorf("save movie artwork: %w", err)
	}

	var subTask *SubtitleTask
	if service.subtitles != nil {
		subTask = service.subtitleTask(input.MetadataPayload, videos)
	}

	if service.notifier != nil {
		service.notifier.NotifyLibraryChanged()
	}
	return subTask, nil
}

// subtitleTask targets the .strm exported for a movie's video. Multi-part
// movies export one .strm per part, and whole-movie subtitles fit none of them.
func (service *Service) subtitleTask(input MetadataPayload, videos []pan.File) *SubtitleTask {
	if len(videos) != 1 {
		return nil
	}
	return &SubtitleTask{
		MovieID:     input.MovieID,
		MetaPayload: input,
		Target: subtitle.Target{
			Dir:           EmbyMovieDir(service.embyDir, input.Code),
			Stem:          nfo.FileStem(input.Code),
			Code:          input.Code,
			Uncensored:    subtitle.IsUncensored(videos[0].Name),
			HardSubtitled: subtitle.HasHardSubtitle(videos[0].Name),
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

func (service *Service) originImage(ctx context.Context, sess drive.Session, entry pan.File) ([]byte, error) {
	info, err := drive.SourceInfo(ctx, sess, entry.ID)
	if err != nil {
		return nil, fmt.Errorf("find NFO artwork: %w", err)
	}
	return sess.Read(ctx, info.File.PickCode, 32<<20)
}

func (service *Service) writeSidecars(ctx context.Context, sess drive.Session, input CoverPayload, directory MovieDirectory, dirVideos []pan.File, poster, fanart []byte) (DirectorySnapshot, error) {
	var snapshot DirectorySnapshot
	stem := nfo.FileStem(input.Code)
	nfoName := stem + ".nfo"

	doc := input.Document
	// An existing matching NFO is already the source of truth. Preserve its
	// formatting and user edits, as well as its referenced artwork.
	if existingDoc, origin, found, err := DirectoryNFO(ctx, sess, input.Code, directory); err != nil {
		return snapshot, err
	} else if found {
		if err := VerifyCoverOrigin(input, directory.ID, existingDoc, *origin, poster, fanart); err != nil {
			return snapshot, err
		}
		doc = existingDoc
	}

	posterName, fanartName := "poster.jpg", "fanart.jpg"
	doc.Thumbs = []nfo.Thumb{{Aspect: "poster", Path: posterName}}
	doc.Fanart = fanartName

	if err := service.exportLocalMedia(ctx, input, stem, doc, dirVideos, poster, fanart); err != nil {
		return snapshot, err
	}

	body, err := nfo.Encode(doc)
	if err != nil {
		return snapshot, err
	}

	return NewDirectorySnapshot(directory.ID,
		pan.File{Name: nfoName, SHA1: pan.SHA1(body)},
		pan.File{Name: posterName, SHA1: pan.SHA1(poster)},
		pan.File{Name: fanartName, SHA1: pan.SHA1(fanart)}), nil
}

func (service *Service) exportLocalMedia(ctx context.Context, input CoverPayload, stem string, doc nfo.Movie, videos []pan.File, poster, fanart []byte) error {
	// Resolve videos from database if empty
	if len(videos) == 0 && service.db != nil && input.MovieID > 0 {
		records, _ := service.db.File.Query().
			Where(file.MovieIDEQ(input.MovieID)).
			Order(ent.Asc(file.FieldName), ent.Asc(file.FieldID)).
			All(ctx)
		for _, r := range records {
			videos = append(videos, pan.File{ID: r.FileID, Name: r.Name, Size: r.Size, PickCode: r.PickCode})
		}
	}

	if err := ExportEmbyMedia(service.embyDir, service.publicURL, service.strmToken, input.Code, doc, videos, poster, fanart); err != nil {
		return err
	}
	if service.mediaNotifier != nil && service.embyDir != "" {
		service.mediaNotifier.NotifyUpdated(EmbyMovieDir(service.embyDir, input.Code))
	}
	return nil
}

// VerifyCoverOrigin validates that existing sidecars have not changed concurrently.
func VerifyCoverOrigin(input CoverPayload, directoryID string, current nfo.Movie, origin ArtworkOrigin, poster, fanart []byte) error {
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
		return domain.E(domain.KindConflict, "NFO 或图片在处理期间发生变化，请重新扫描", nil)
	}
	return nil
}

// UploadSidecar writes a sidecar file into a movie's directory on 115 storage.
func UploadSidecar(ctx context.Context, sess drive.Session, directory MovieDirectory, name string, body []byte) error {
	for _, entry := range directory.Files {
		if !directory.VideoIDs[entry.ID] {
			continue
		}
		info, err := sess.Info(ctx, entry.ID)
		if err != nil {
			return err
		}
		if info.ParentID != directory.ID || !drive.WithinSource(info, sess.Source()) {
			return domain.E(domain.KindConflict, "视频已移动，请重新扫描", nil)
		}
		return sess.Upload(ctx, directory.ID, name, body)
	}
	return domain.E(domain.KindInvalid, "没有可写入元数据的视频目录", nil)
}
