package service

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/tag"
	"github.com/ppxb/miyabi/internal/ent/task"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
)

type metadataPayload struct {
	Source     LibrarySource `json:"source"`
	ScanTaskID int           `json:"scan_task_id"`
	MovieID    int           `json:"movie_id"`
	Code       string        `json:"code"`
	JavDBID    string        `json:"javdb_id,omitempty"`
}

type artworkOrigin struct {
	NFO    pan.File `json:"nfo"`
	Poster pan.File `json:"poster"`
	Fanart pan.File `json:"fanart"`
}

type coverPayload struct {
	metadataPayload
	ScrapeTaskID int                 `json:"scrape_task_id"`
	Document     nfo.Movie           `json:"document"`
	CoverURL     string              `json:"cover_url,omitempty"`
	Origin       *artworkOrigin      `json:"origin,omitempty"`
	Artwork      *mediaimage.Artwork `json:"artwork,omitempty"`
	Snapshot     *metadataSnapshot   `json:"snapshot,omitempty"`
}

type movieDirectory struct {
	ID       string
	Files    []pan.File
	VideoIDs map[string]bool
	Shared   bool
}

type ScrapeService struct {
	library  *LibraryService
	discover *DiscoverService
	images   *mediaimage.Cache
}

func NewScrapeService(library *LibraryService, discover *DiscoverService, images *mediaimage.Cache) *ScrapeService {
	return &ScrapeService{library: library, discover: discover, images: images}
}

func (service *ScrapeService) begin(ctx context.Context, input metadataPayload) (uint64, error) {
	service.library.drive.mu.Lock()
	defer service.library.drive.mu.Unlock()
	source, err := service.library.verifiedSource(ctx)
	if err != nil {
		return 0, err
	}
	if source.AccountID != input.Source.AccountID || source.Directory.ID != input.Source.Directory.ID {
		return 0, fmt.Errorf("媒体目录或登录账号已变更，请重新扫描")
	}
	return service.library.drive.authorizationVersion, nil
}

func (service *ScrapeService) Scrape(ctx context.Context, job TaskJob) error {
	input, err := decodeTaskPayload[metadataPayload](job.Payload)
	if err != nil {
		return err
	}
	// A committed cover job means the metadata transaction already succeeded.
	queued, err := service.library.database.Task.Query().Where(task.TypeEQ("cover"), func(s *sql.Selector) {
		s.Where(sqljson.ValueEQ(task.FieldPayload, job.ID, sqljson.Path("scrape_task_id")))
	}).Exist(ctx)
	if err != nil {
		return fmt.Errorf("find queued artwork: %w", err)
	}
	if queued {
		return nil
	}
	version, err := service.begin(ctx, input)
	if err != nil {
		return err
	}
	record, err := service.library.database.Movie.Query().Where(movie.IDEQ(input.MovieID),
		movie.HasFilesWith(libraryFiles(input.Source))).WithActors().WithTags().Only(ctx)
	if err != nil {
		return fmt.Errorf("load indexed movie for metadata: %w", err)
	}
	directories, err := service.directories(ctx, input, version)
	if err != nil {
		return err
	}
	cover := coverPayload{metadataPayload: input, ScrapeTaskID: job.ID}
	for _, directory := range directories {
		doc, origin, found, err := service.directoryNFO(ctx, input, version, directory)
		if err != nil {
			return err
		}
		if found {
			cover.Document, cover.Origin = doc, origin
			break
		}
	}
	if cover.Document.Code == "" {
		if record.ScrapeStatus == movie.ScrapeStatusDone {
			cover.Document = movieNFO(record)
			artwork := movieArtwork(record)
			cover.Artwork = &artwork
		} else {
			id := input.JavDBID
			if id == "" {
				id = valueOrZero(record.JavdbID)
			}
			if id == "" {
				id, err = service.discover.ResolveMovieID(ctx, input.Code)
				if err != nil {
					return err
				}
			}
			detail, err := service.discover.MovieDetail(ctx, id)
			if err != nil {
				return err
			}
			if codeid.Normalize(detail.Code) != input.Code {
				return fmt.Errorf("JavDB 返回的番号 %s 与媒体文件 %s 不一致", detail.Code, input.Code)
			}
			if err := service.completeTagCategories(ctx, &detail); err != nil {
				return err
			}
			cover.Document = detailNFO(detail)
			cover.Document.Code = input.Code
			cover.CoverURL = detail.Cover
		}
	}
	encoded, err := encodeTaskPayload(cover)
	if err != nil {
		return err
	}
	service.library.drive.mu.Lock()
	defer service.library.drive.mu.Unlock()
	if err := service.library.checkScanSource(input.Source, version); err != nil {
		return err
	}
	if err := ent.WithTx(ctx, service.library.database, func(tx *ent.Tx) error {
		if err := saveMovieMetadata(ctx, tx, input.MovieID, cover.Document); err != nil {
			return err
		}
		return tx.Task.Create().SetType("cover").SetPayload(encoded).Exec(ctx)
	}); err != nil {
		return fmt.Errorf("save movie metadata and queue artwork: %w", err)
	}
	service.library.tasks.NotifyLibraryChanged()
	return nil
}

func (service *ScrapeService) directories(ctx context.Context, input metadataPayload, version uint64) ([]movieDirectory, error) {
	files, err := service.library.database.File.Query().Where(libraryFiles(input.Source), file.MovieIDEQ(input.MovieID)).
		Order(ent.Asc(file.FieldParentID), ent.Asc(file.FieldFileID)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load movie file directories: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("影片已没有媒体文件，请重新扫描")
	}
	var result []movieDirectory
	byID := make(map[string]int)
	for _, entry := range files {
		index, found := byID[entry.ParentID]
		if !found {
			index = len(result)
			byID[entry.ParentID] = index
			result = append(result, movieDirectory{ID: entry.ParentID, VideoIDs: make(map[string]bool)})
		}
		result[index].VideoIDs[entry.FileID] = true
	}
	for i := range result {
		directory := &result[i]
		directory.Files, err = service.library.directoryEntries(ctx, input.Source, version, directory.ID)
		if err != nil {
			return nil, fmt.Errorf("read metadata directory: %w", err)
		}
		present := 0
		for _, entry := range directory.Files {
			if !entry.IsDirectory && isVideo(entry.Name) {
				if directory.VideoIDs[entry.ID] {
					present++
				} else {
					directory.Shared = true
				}
			}
		}
		if present == 0 {
			return nil, fmt.Errorf("视频文件已删除或移动，请重新扫描")
		}
	}
	return result, nil
}

func findDirectoryNFO(code string, directory movieDirectory) (pan.File, bool) {
	entry, found := sidecarByName(directory.Files, nfo.FileStem(code)+".nfo")
	if !found {
		var candidates []pan.File
		for _, item := range directory.Files {
			if !item.IsDirectory && strings.EqualFold(path.Ext(item.Name), ".nfo") {
				candidateCode, _ := codeid.Parse(item.Name)
				if candidateCode == code {
					entry, found = item, true
					break
				}
				candidates = append(candidates, item)
			}
		}
		if !found && !directory.Shared && len(candidates) == 1 {
			entry, found = candidates[0], true
		}
	}
	return entry, found
}

func (service *ScrapeService) directoryNFO(ctx context.Context, input metadataPayload, version uint64, directory movieDirectory) (nfo.Movie, *artworkOrigin, bool, error) {
	entry, found := findDirectoryNFO(input.Code, directory)
	if !found {
		return nfo.Movie{}, nil, false, nil
	}
	body, err := service.library.readSidecar(ctx, input.Source, version, entry, 2<<20)
	if err != nil {
		return nfo.Movie{}, nil, false, fmt.Errorf("read %s: %w", entry.Name, err)
	}
	doc, err := nfo.Decode(body)
	if err != nil {
		return nfo.Movie{}, nil, false, err
	}
	code := codeid.Normalize(doc.Code)
	if code == "" {
		code, _ = codeid.Parse(entry.Name)
	}
	if code != input.Code {
		return nfo.Movie{}, nil, false, fmt.Errorf("NFO %s 的番号与视频不一致", entry.Name)
	}
	doc.Code = code
	posterName, fanartName := doc.Poster(), doc.Fanart
	if posterName == "" {
		posterName = "poster.jpg"
	}
	if fanartName == "" {
		fanartName = "fanart.jpg"
	}
	// NFO artwork is restored from the same 115 directory, never an arbitrary URL.
	poster, posterFound := sidecarByName(directory.Files, posterName)
	fanart, fanartFound := sidecarByName(directory.Files, fanartName)
	if !posterFound || !fanartFound {
		return nfo.Movie{}, nil, false, fmt.Errorf("NFO %s 对应的海报或封面不存在", entry.Name)
	}
	return doc, &artworkOrigin{NFO: entry, Poster: poster, Fanart: fanart}, true, nil
}

func (service *ScrapeService) completeTagCategories(ctx context.Context, detail *DiscoverMovieDetail) error {
	missing := false
	for _, item := range detail.Tags {
		missing = missing || item.CategoryID == ""
	}
	if !missing {
		return nil
	}
	categories, err := service.discover.Tags(ctx, detail.Zone)
	if err != nil {
		return err
	}
	ids := make(map[string]string)
	for _, category := range categories {
		for _, item := range category.Tags {
			ids[item.ID] = category.ID
		}
	}
	for i := range detail.Tags {
		if detail.Tags[i].CategoryID == "" {
			detail.Tags[i].CategoryID = ids[detail.Tags[i].ID]
			if detail.Tags[i].CategoryID == "" {
				return fmt.Errorf("JavDB 标签 %s 缺少分类", detail.Tags[i].ID)
			}
		}
	}
	return nil
}

func detailNFO(detail DiscoverMovieDetail) nfo.Movie {
	doc := nfo.Movie{Title: detail.Title, Code: detail.Code, Premiered: detail.ReleaseDate,
		Runtime: detail.Duration, Rating: detail.Rating,
		IDs: []nfo.UniqueID{{Type: "javdb", Default: true, Value: detail.ID}},
	}
	if detail.Director != nil {
		doc.Director = nfo.Entity{ID: detail.Director.ID, Name: detail.Director.Name}
	}
	if detail.Maker != nil {
		doc.Studio = nfo.Entity{ID: detail.Maker.ID, Name: detail.Maker.Name}
	}
	if detail.Series != nil {
		doc.Set = nfo.Series{ID: detail.Series.ID, Name: detail.Series.Name}
	}
	for _, person := range detail.Actors {
		doc.Actors = append(doc.Actors, nfo.Actor{ID: person.ID, Name: person.Name,
			NameZHT: person.NameZHT, Gender: person.Gender, Thumb: person.Avatar})
	}
	for _, item := range detail.Tags {
		doc.Tags = append(doc.Tags, nfo.Tag{ID: item.ID, Name: item.Name, NameZHT: item.NameZHT, CategoryID: item.CategoryID})
		doc.Genres = append(doc.Genres, item.Name)
	}
	return doc
}

func movieNFO(record *ent.Movie) nfo.Movie {
	doc := nfo.Movie{Title: record.Title, Code: record.Code, Runtime: valueOrZero(record.Duration), Rating: valueOrZero(record.Rating),
		Director: nfo.Entity{ID: valueOrZero(record.DirectorID), Name: valueOrZero(record.DirectorName)},
		Studio:   nfo.Entity{ID: valueOrZero(record.MakerID), Name: valueOrZero(record.MakerName)},
		Set:      nfo.Series{ID: valueOrZero(record.SeriesID), Name: valueOrZero(record.SeriesName)},
	}
	if record.JavdbID != nil {
		doc.IDs = []nfo.UniqueID{{Type: "javdb", Default: true, Value: *record.JavdbID}}
	}
	if record.ReleaseDate != nil {
		doc.Premiered = record.ReleaseDate.Format(time.DateOnly)
	}
	for _, person := range record.Edges.Actors {
		doc.Actors = append(doc.Actors, nfo.Actor{ID: person.JavdbID, Name: person.Name,
			NameZHT: valueOrZero(person.NameZht), Gender: string(person.Gender), Thumb: valueOrZero(person.Avatar)})
	}
	for _, item := range record.Edges.Tags {
		doc.Tags = append(doc.Tags, nfo.Tag{ID: item.JavdbID, Name: item.Name,
			NameZHT: valueOrZero(item.NameZht), CategoryID: item.CategoryID})
		doc.Genres = append(doc.Genres, item.Name)
	}
	return doc
}

func valueOrZero[T any](value *T) T {
	if value != nil {
		return *value
	}
	var zero T
	return zero
}

func saveMovieMetadata(ctx context.Context, tx *ent.Tx, id int, doc nfo.Movie) error {
	update := tx.Movie.UpdateOneID(id).SetTitle(doc.Title).SetScrapeStatus(movie.ScrapeStatusPending).
		ClearActors().ClearTags().ClearJavdbID().ClearReleaseDate().ClearDuration().ClearRating().
		ClearDirectorID().ClearDirectorName().ClearMakerID().ClearMakerName().ClearSeriesID().ClearSeriesName()
	if doc.JavDBID() != "" {
		update.SetJavdbID(doc.JavDBID())
	}
	if doc.Premiered != "" {
		date, err := time.Parse(time.DateOnly, doc.Premiered)
		if err != nil {
			return fmt.Errorf("parse metadata release date: %w", err)
		}
		update.SetReleaseDate(date)
	}
	if doc.Runtime > 0 {
		update.SetDuration(doc.Runtime)
	}
	if doc.Rating > 0 {
		update.SetRating(doc.Rating)
	}
	if doc.Director.ID != "" {
		update.SetDirectorID(doc.Director.ID)
	}
	if doc.Director.Name != "" {
		update.SetDirectorName(doc.Director.Name)
	}
	if doc.Studio.ID != "" {
		update.SetMakerID(doc.Studio.ID)
	}
	if doc.Studio.Name != "" {
		update.SetMakerName(doc.Studio.Name)
	}
	if doc.Set.ID != "" {
		update.SetSeriesID(doc.Set.ID)
	}
	if doc.Set.Name != "" {
		update.SetSeriesName(doc.Set.Name)
	}
	actors := make([]*ent.ActorCreate, 0, len(doc.Actors))
	actorIDs := make([]string, 0, len(doc.Actors))
	for _, person := range doc.Actors {
		// Standard third-party NFOs may not carry stable JavDB IDs. Do not
		// manufacture IDs from names or contact JavDB to fill them in.
		if person.ID == "" {
			continue
		}
		gender := actor.Gender(person.Gender)
		if gender == "" {
			gender = actor.GenderUnknown
		}
		actors = append(actors, tx.Actor.Create().SetJavdbID(person.ID).SetName(person.Name).
			SetNameZht(person.NameZHT).SetGender(gender).SetAvatar(person.Thumb))
		actorIDs = append(actorIDs, person.ID)
	}
	if len(actors) > 0 {
		if err := tx.Actor.CreateBulk(actors...).OnConflictColumns(actor.FieldJavdbID).UpdateNewValues().Exec(ctx); err != nil {
			return err
		}
		ids, err := tx.Actor.Query().Where(actor.JavdbIDIn(actorIDs...)).IDs(ctx)
		if err != nil {
			return err
		}
		update.AddActorIDs(ids...)
	}
	tags := make([]*ent.TagCreate, 0, len(doc.Tags))
	tagIDs := make([]string, 0, len(doc.Tags))
	for _, item := range doc.Tags {
		if item.ID == "" || item.CategoryID == "" {
			continue
		}
		tags = append(tags, tx.Tag.Create().SetJavdbID(item.ID).SetName(item.Name).SetNameZht(item.NameZHT).SetCategoryID(item.CategoryID))
		tagIDs = append(tagIDs, item.ID)
	}
	if len(tags) > 0 {
		if err := tx.Tag.CreateBulk(tags...).OnConflictColumns(tag.FieldJavdbID).UpdateNewValues().Exec(ctx); err != nil {
			return err
		}
		ids, err := tx.Tag.Query().Where(tag.JavdbIDIn(tagIDs...)).IDs(ctx)
		if err != nil {
			return err
		}
		update.AddTagIDs(ids...)
	}
	return update.Exec(ctx)
}
