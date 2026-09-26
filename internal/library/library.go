package library

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/predicate"
	"github.com/ppxb/miyabi/internal/ent/tag"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/library/scan"
	"github.com/ppxb/miyabi/internal/tasks"
)

type Entity struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

type Tag struct {
	ID      int    `json:"id"`
	JavDBID string `json:"javdb_id"`
	Name    string `json:"name"`
}

type Movie struct {
	ID           int                `json:"id"`
	Code         string             `json:"code"`
	Title        string             `json:"title"`
	JavDBID      *string            `json:"javdb_id,omitempty"`
	Cover        *string            `json:"cover,omitempty"`
	Poster       *string            `json:"poster,omitempty"`
	Fanart       string             `json:"fanart,omitempty"`
	ReleaseDate  string             `json:"release_date,omitempty"`
	Duration     int                `json:"duration"`
	Rating       float64            `json:"rating"`
	Director     *Entity            `json:"director,omitempty"`
	Maker        *Entity            `json:"maker,omitempty"`
	Series       *Entity            `json:"series,omitempty"`
	Actors       []Entity           `json:"actors"`
	Tags         []Tag              `json:"tags"`
	ScrapeStatus movie.ScrapeStatus `json:"scrape_status"`
}

type Page struct {
	Source  *domain.LibrarySource `json:"source,omitempty"`
	Movies  []Movie               `json:"movies"`
	Total   int                   `json:"total"`
	Page    int                   `json:"page"`
	HasMore bool                  `json:"has_more"`
}

type Service struct {
	images       *mediaimage.Cache
	database     *ent.Client
	drive        *drive.Drive
	tasks        *tasks.Service
	scanner      *scan.Scanner
	localScanner *scan.LocalScanner
}

func New(database *ent.Client, d *drive.Drive, tasks *tasks.Service, images *mediaimage.Cache) *Service {
	svc := &Service{
		database:     database,
		drive:        d,
		tasks:        tasks,
		images:       images,
		scanner:      scan.New(d, database, images, tasks),
		localScanner: scan.NewLocalScanner(database, images),
	}
	if d != nil && tasks != nil {
		d.SubscribeMount(func(ctx context.Context, event drive.MountEvent) error {
			if event.Source.Directory.ID != "" {
				if _, err := tasks.EnqueueFreshScan(ctx, event.Source); err != nil {
					return err
				}
			}
			tasks.NotifyLibraryChanged()
			return nil
		})
	}
	return svc
}

func (s *Service) StartScan(ctx context.Context) (tasks.TaskInfo, error) {
	sess, err := s.drive.Open(ctx)
	if err != nil {
		return tasks.TaskInfo{}, err
	}
	return s.tasks.EnqueueScan(ctx, sess.Source())
}

// Source returns the currently mounted library source, or nil if unmounted.
func (s *Service) Source() *domain.LibrarySource {
	if s.drive == nil {
		return nil
	}
	return s.drive.Source()
}

// MatchingMovies queries local movies matching any of the given JavDB IDs or normalized codes
// that possess indexed files in the currently mounted library source or local library.
func (s *Service) MatchingMovies(ctx context.Context, javdbIDs []string, codes []string) ([]domain.LocalMovie, error) {
	if len(javdbIDs) == 0 && len(codes) == 0 {
		return nil, nil
	}
	source := s.Source()
	var filePredicate predicate.File
	if source != nil {
		filePredicate = file.Or(database.LibraryFiles(*source), file.AccountIDEQ("local"))
	} else {
		filePredicate = file.AccountIDEQ("local")
	}
	records, err := s.database.Movie.Query().Where(
		movie.Or(movie.JavdbIDIn(javdbIDs...), movie.And(movie.JavdbIDIsNil(), movie.CodeIn(codes...))),
		movie.HasFilesWith(filePredicate),
	).Select(movie.FieldID, movie.FieldCode, movie.FieldJavdbID).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query matching movies: %w", err)
	}
	result := make([]domain.LocalMovie, len(records))
	for i, r := range records {
		result[i] = domain.LocalMovie{
			ID:      r.ID,
			Code:    r.Code,
			JavDBID: r.JavdbID,
		}
	}
	return result, nil
}

// ScanLocal scans a local directory for media and sidecars and imports them.
func (s *Service) ScanLocal(ctx context.Context, rootDir string) (*scan.LocalScanResult, error) {
	result, err := s.localScanner.Scan(ctx, rootDir)
	if err != nil {
		return nil, err
	}
	if s.tasks != nil {
		s.tasks.NotifyLibraryChanged()
	}
	return result, nil
}

func (s *Service) SetEmbyExport(embyDir, publicURL, strmToken string) {
	s.scanner.SetEmbyExport(embyDir, publicURL, strmToken)
}

func (s *Service) SetPacing(pace func(context.Context) error) {
	s.scanner.SetPacing(pace)
}

func (s *Service) SetMediaNotifier(notifier scan.MediaNotifier) {
	s.scanner.SetMediaNotifier(notifier)
	s.localScanner.SetMediaNotifier(notifier)
}

func (s *Service) Scan(ctx context.Context, job tasks.Job) error {
	return s.scanner.Run(ctx, job)
}

func (s *Service) Finished(context.Context, *ent.Tx, tasks.Job, error) (tasks.Change, error) {
	return tasks.ChangeOffline, nil
}

// EnqueueTargetedScan creates a targeted scan task inside the caller's transaction.
func (s *Service) EnqueueTargetedScan(ctx context.Context, tx *ent.Tx, source domain.LibrarySource, targetID string, offlineTaskID int, code, javdbID string) (int, error) {
	encoded, err := tasks.EncodePayload(scan.Payload{
		Source:        source,
		Scan:          domain.ScanProgress{Stage: "queued", CurrentPath: source.Directory.Path},
		TargetID:      targetID,
		OfflineTaskID: offlineTaskID,
		Code:          code,
		JavDBID:       javdbID,
	})
	if err != nil {
		return 0, err
	}
	taskRecord, err := tx.Task.Create().
		SetType(tasks.KindScan.String()).
		SetPayload(encoded).
		Save(ctx)
	if err != nil {
		return 0, err
	}
	return taskRecord.ID, nil
}

func (s *Service) Movies(ctx context.Context, page, limit int) (Page, error) {
	result := Page{Movies: []Movie{}, Page: page}
	source := s.drive.Source()
	var scope predicate.File
	if source != nil {
		result.Source = source
		scope = file.Or(database.LibraryFiles(*source), file.AccountIDEQ("local"))
	} else {
		scope = file.AccountIDEQ("local")
	}
	var err error
	result.Total, err = s.database.File.Query().Where(scope).Aggregate(func(selector *sql.Selector) string {
		return sql.As("COUNT(DISTINCT "+selector.C(file.FieldMovieID)+")", "total")
	}).Int(ctx)
	if err != nil {
		return result, fmt.Errorf("count library index: %w", err)
	}
	if result.Total == 0 {
		return result, nil
	}
	records, err := s.database.Movie.Query().Where(movie.HasFilesWith(scope)).
		Select(movie.FieldID, movie.FieldCode, movie.FieldTitle, movie.FieldJavdbID, movie.FieldCover, movie.FieldPoster,
			movie.FieldFanarts, movie.FieldReleaseDate, movie.FieldDuration, movie.FieldRating,
			movie.FieldDirectorID, movie.FieldDirectorName, movie.FieldMakerID, movie.FieldMakerName,
			movie.FieldSeriesID, movie.FieldSeriesName,
			movie.FieldScrapeStatus).
		Order(ent.Desc(movie.FieldCreatedAt), ent.Desc(movie.FieldID)).
		Offset((page - 1) * limit).Limit(limit).
		WithActors(func(query *ent.ActorQuery) {
			query.Select(actor.FieldID, actor.FieldJavdbID, actor.FieldName).Order(ent.Asc(actor.FieldName), ent.Asc(actor.FieldID))
		}).
		WithTags(func(query *ent.TagQuery) {
			query.Select(tag.FieldID, tag.FieldJavdbID, tag.FieldName).Order(ent.Asc(tag.FieldName), ent.Asc(tag.FieldID))
		}).All(ctx)
	if err != nil {
		return result, fmt.Errorf("list library movies: %w", err)
	}
	for _, record := range records {
		item := Movie{
			ID: record.ID, Code: record.Code, Title: record.Title,
			JavDBID: record.JavdbID, Cover: record.Cover, Poster: record.Poster,
			Duration: domain.ValueOrZero(record.Duration), Rating: domain.ValueOrZero(record.Rating),
			Director: libraryEntity(record.DirectorID, record.DirectorName),
			Maker:    libraryEntity(record.MakerID, record.MakerName), Series: libraryEntity(record.SeriesID, record.SeriesName),
			Actors: make([]Entity, 0, len(record.Edges.Actors)),
			Tags:   make([]Tag, 0, len(record.Edges.Tags)), ScrapeStatus: record.ScrapeStatus,
		}
		if record.ReleaseDate != nil {
			item.ReleaseDate = record.ReleaseDate.Format(time.DateOnly)
		}
		if len(record.Fanarts) > 0 {
			item.Fanart = record.Fanarts[0]
		}
		for _, person := range record.Edges.Actors {
			item.Actors = append(item.Actors, Entity{ID: person.JavdbID, Name: person.Name})
		}
		for _, label := range record.Edges.Tags {
			item.Tags = append(item.Tags, Tag{ID: label.ID, JavDBID: label.JavdbID, Name: label.Name})
		}
		result.Movies = append(result.Movies, item)
	}
	result.HasMore = (page-1)*limit+len(result.Movies) < result.Total
	return result, nil
}

func libraryEntity(id, name *string) *Entity {
	if name == nil || *name == "" {
		return nil
	}
	return &Entity{ID: domain.ValueOrZero(id), Name: *name}
}
