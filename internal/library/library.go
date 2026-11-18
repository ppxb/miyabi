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
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/export"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/library/scan"
	"github.com/ppxb/miyabi/internal/syncx"
	"github.com/ppxb/miyabi/internal/tasks"
)

type Entity struct {
	Provider string `json:"provider,omitempty"`
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
}

type Tag struct {
	ID       int    `json:"id"`
	Provider string `json:"provider"`
	SourceID string `json:"source_id"`
	Name     string `json:"name"`
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
	// Source marks movies that have at least one file in a 115 cloud source.
	Source string `json:"source,omitempty"`
}

type Page struct {
	Source  *domain.LibrarySource `json:"source,omitempty"`
	Movies  []Movie               `json:"movies"`
	Total   int                   `json:"total"`
	Page    int                   `json:"page"`
	HasMore bool                  `json:"has_more"`
}

type Service struct {
	database     *ent.Client
	drive        *drive.Drive
	tasks        *tasks.Service
	scanner      *scan.Scanner
	localScanner *scan.LocalScanner
	exportMgr    *export.Manager
	scanLock     syncx.ContextLock
	sources      func(context.Context) ([]domain.LibrarySource, error)
	localSources func(context.Context) ([]LocalScanTarget, error)
}

// LocalScanTarget is an enabled local source directory and its metadata mode.
type LocalScanTarget struct {
	Path         string
	MetadataOnly bool
}

type Options struct {
	ExportManager *export.Manager
	// Sources supplies the enabled cloud sources to aggregate in the library.
	Sources func(context.Context) ([]domain.LibrarySource, error)
	// LocalSources supplies the enabled local source directories.
	LocalSources func(context.Context) ([]LocalScanTarget, error)
	// Pacing defaults to scan.DefaultPacing; tests can provide a no-op.
	Pacing func(context.Context) error
}

func New(database *ent.Client, d *drive.Drive, tasks *tasks.Service, images *mediaimage.Cache, options Options) *Service {
	svc := &Service{
		database:     database,
		drive:        d,
		tasks:        tasks,
		scanner:      scan.New(d, database, images, tasks, options.ExportManager, options.Pacing),
		localScanner: scan.NewLocalScanner(database, images),
		exportMgr:    options.ExportManager,
		sources:      options.Sources,
		localSources: options.LocalSources,
	}
	if d != nil && tasks != nil {
		d.SetMountListener(func(ctx context.Context, event drive.MountEvent) error {
			if event.Source.Directory.ID != "" {
				if _, err := svc.EnqueueFreshScan(ctx, event.Source); err != nil {
					return err
				}
			}
			tasks.NotifyLibraryChanged()
			return nil
		})
	}
	return svc
}

func (s *Service) StartScan(ctx context.Context) (domain.TaskInfo, error) {
	sess, err := s.drive.Open(ctx)
	if err != nil {
		return domain.TaskInfo{}, err
	}
	return s.EnqueueScan(ctx, sess.Source())
}

// StartRebuild scans the current source and refreshes every matched movie.
func (s *Service) StartRebuild(ctx context.Context) (domain.TaskInfo, error) {
	sess, err := s.drive.Open(ctx)
	if err != nil {
		return domain.TaskInfo{}, err
	}
	return s.enqueueScan(ctx, sess.Source(), true, task.StatusQueued, task.StatusRunning)
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
	records, err := s.database.Movie.Query().Where(
		movie.Or(movie.JavdbIDIn(javdbIDs...), movie.And(movie.JavdbIDIsNil(), movie.CodeIn(codes...))),
		movie.HasFilesWith(s.scope(ctx)),
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

// SetMediaNotifier binds the library/Emby cycle before task workers start.
func (s *Service) SetMediaNotifier(notifier export.MediaNotifier) {
	s.scanner.SetMediaNotifier(notifier)
	s.localScanner.SetMediaNotifier(notifier)
}

func (s *Service) Scan(ctx context.Context, job tasks.Job) error {
	if err := s.scanLock.Lock(ctx); err != nil {
		return err
	}
	defer s.scanLock.Unlock()
	payload, err := tasks.DecodePayload[domain.ScanPayload](job.Payload)
	if err != nil {
		return err
	}
	if payload.Source.AccountID == domain.LocalAccountID {
		// Emby imports must not observe partially rewritten STRM or exported sidecars;
		// user-configured local sources are independent of the export directory.
		if payload.Source.Directory.Name == embyLocalDirectoryName {
			return s.exportMgr.WithConfig(func(export.Config) error {
				return s.scanLocal(ctx, job.ID, payload)
			})
		}
		return s.scanLocal(ctx, job.ID, payload)
	}
	return s.scanner.Run(ctx, job)
}

func (s *Service) Finished(_ context.Context, _ *ent.Tx, job tasks.Job, _ error) (tasks.Change, error) {
	payload, err := tasks.DecodePayload[domain.ScanPayload](job.Payload)
	if err == nil && payload.Source.AccountID == domain.LocalAccountID {
		return tasks.ChangeLibrary, nil
	}
	return tasks.ChangeOffline, nil
}

// EnqueueTargetedScan creates a targeted scan task inside the caller's transaction.
func (s *Service) EnqueueTargetedScan(ctx context.Context, tx *ent.Tx, source domain.LibrarySource, targetID string, offlineTaskID int, code, javdbID string) (int, error) {
	encoded, err := tasks.EncodePayload(domain.ScanPayload{
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
	tx.OnCommit(func(next ent.Committer) ent.Committer {
		return ent.CommitFunc(func(ctx context.Context, tx *ent.Tx) error {
			if err := next.Commit(ctx, tx); err != nil {
				return err
			}
			s.tasks.WakePool()
			return nil
		})
	})
	return taskRecord.ID, nil
}

func libraryScope(source *domain.LibrarySource) predicate.File {
	local := file.AccountIDEQ(domain.LocalAccountID)
	if source != nil {
		return file.Or(database.LibraryFiles(*source), local)
	}
	return local
}

// StartIncrementalSync queues an incremental scan for every enabled source.
func (s *Service) StartIncrementalSync(ctx context.Context) (int, error) {
	started := 0
	if s.localSources != nil {
		targets, err := s.localSources(ctx)
		if err != nil {
			return started, err
		}
		for _, target := range targets {
			if _, err := s.StartLocalSourceScanModeIncremental(ctx, target.Path, target.MetadataOnly); err != nil {
				return started, err
			}
			started++
		}
	}
	if s.sources != nil {
		list, err := s.sources(ctx)
		if err != nil {
			return started, err
		}
		for _, source := range list {
			if _, err := s.EnqueueSyncMode(ctx, source, false); err != nil {
				return started, err
			}
			started++
		}
	}
	return started, nil
}

// PendingCount reports movies across the aggregated sources still awaiting metadata.
func (s *Service) PendingCount(ctx context.Context) (int, error) {
	return s.database.Movie.Query().
		Where(movie.HasFilesWith(s.scope(ctx)), movie.ScrapeStatusEQ(movie.ScrapeStatusPending)).
		Count(ctx)
}

// scope aggregates files across the primary mount, configured cloud sources and
// the local library, so every enabled source shows up in the media library.
func (s *Service) scope(ctx context.Context) predicate.File {
	return fileScope(s.cloudSources(ctx))
}

// cloudSources returns the primary mount plus every configured cloud source.
func (s *Service) cloudSources(ctx context.Context) []domain.LibrarySource {
	sources := make([]domain.LibrarySource, 0)
	if source := s.Source(); source != nil {
		sources = append(sources, *source)
	}
	if s.sources != nil {
		if list, err := s.sources(ctx); err == nil {
			sources = append(sources, list...)
		}
	}
	return sources
}

func fileScope(sources []domain.LibrarySource) predicate.File {
	scopes := []predicate.File{file.AccountIDEQ(domain.LocalAccountID)}
	for _, source := range sources {
		scopes = append(scopes, database.LibraryFiles(source))
	}
	return file.Or(scopes...)
}

// cloudFileScope matches files that belong to one of the cloud sources, the
// same account and root pair scope uses to filter the library.
func cloudFileScope(sources []domain.LibrarySource) predicate.File {
	preds := make([]predicate.File, 0, len(sources))
	for _, source := range sources {
		if source.AccountID == "" || source.AccountID == domain.LocalAccountID {
			continue
		}
		preds = append(preds, database.LibraryFiles(source))
	}
	if len(preds) == 0 {
		return nil
	}
	return file.Or(preds...)
}

func (s *Service) Movies(ctx context.Context, page, limit int) (Page, error) {
	result := Page{Movies: []Movie{}, Page: page, Source: s.Source()}
	sources := s.cloudSources(ctx)
	scope := fileScope(sources)
	cloudScope := cloudFileScope(sources)
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
	order := []movie.OrderOption{ent.Desc(movie.FieldCreatedAt), ent.Desc(movie.FieldID)}
	if cloudScope != nil {
		// Newest-joined 115 titles first; local-only movies have no 115 join
		// time and fall after them, ordered by their own creation time.
		order = append([]movie.OrderOption{cloudAddedOrder(sources)}, order...)
	}
	records, err := s.database.Movie.Query().Where(movie.HasFilesWith(scope)).
		Select(movie.FieldID, movie.FieldCode, movie.FieldTitle, movie.FieldJavdbID, movie.FieldCover, movie.FieldPoster,
			movie.FieldFanarts, movie.FieldReleaseDate, movie.FieldDuration, movie.FieldRating,
			movie.FieldDirectorID, movie.FieldDirectorName, movie.FieldMakerID, movie.FieldMakerName,
			movie.FieldSeriesID, movie.FieldSeriesName,
			movie.FieldScrapeStatus).
		Order(order...).
		Offset((page - 1) * limit).Limit(limit).
		WithActors(func(query *ent.ActorQuery) {
			query.Select(actor.FieldID, actor.FieldProvider, actor.FieldSourceID, actor.FieldName).Order(ent.Asc(actor.FieldName), ent.Asc(actor.FieldID))
		}).
		WithTags(func(query *ent.TagQuery) {
			query.Select(tag.FieldID, tag.FieldProvider, tag.FieldSourceID, tag.FieldName).Order(ent.Asc(tag.FieldName), ent.Asc(tag.FieldID))
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
			item.Actors = append(item.Actors, Entity{Provider: person.Provider, ID: person.SourceID, Name: person.Name})
		}
		for _, label := range record.Edges.Tags {
			item.Tags = append(item.Tags, Tag{ID: label.ID, Provider: label.Provider, SourceID: label.SourceID, Name: label.Name})
		}
		result.Movies = append(result.Movies, item)
	}
	if err := s.markCloudSources(ctx, result.Movies, cloudScope); err != nil {
		return result, err
	}
	result.HasMore = (page-1)*limit+len(result.Movies) < result.Total
	return result, nil
}

// markCloudSources flags movies that have at least one file in a 115 source.
func (s *Service) markCloudSources(ctx context.Context, movies []Movie, cloudScope predicate.File) error {
	if cloudScope == nil || len(movies) == 0 {
		return nil
	}
	ids := make([]int, len(movies))
	for i := range movies {
		ids[i] = movies[i].ID
	}
	records, err := s.database.File.Query().
		Where(file.MovieIDIn(ids...), cloudScope).
		Select(file.FieldMovieID).All(ctx)
	if err != nil {
		return fmt.Errorf("detect library movie sources: %w", err)
	}
	inCloud := make(map[int]bool, len(records))
	for _, record := range records {
		if record.MovieID != nil {
			inCloud[*record.MovieID] = true
		}
	}
	for i := range movies {
		if inCloud[movies[i].ID] {
			movies[i].Source = "115"
		}
	}
	return nil
}

// cloudAddedOrder orders movies by the newest 115 upload time, newest first.
// upload_time is the 115 "upt" value stored when the file was scanned, so it
// reflects the real "joined 115" order rather than the local scan time.
// Local-only movies have no 115 file and rank after every 115 title.
func cloudAddedOrder(sources []domain.LibrarySource) movie.OrderOption {
	return func(selector *sql.Selector) {
		query := sql.Dialect(selector.Dialect()).String(func(b *sql.Builder) {
			b.S("(SELECT MAX(").Ident(file.Table).S(".").Ident(file.FieldUploadTime).S(") FROM ").Ident(file.Table)
			b.S(" WHERE ").Ident(file.Table).S(".").Ident(file.FieldMovieID).S(" = ").Ident(movie.Table).S(".").Ident(movie.FieldID)
			b.S(" AND (")
			first := true
			for _, source := range sources {
				if source.AccountID == "" || source.AccountID == domain.LocalAccountID {
					continue
				}
				if !first {
					b.S(" OR ")
				}
				first = false
				b.S("(").Ident(file.Table).S(".").Ident(file.FieldAccountID).S(" = ? AND ").
					Ident(file.Table).S(".").Ident(file.FieldRootID).S(" = ?)")
			}
			b.S(")) DESC")
		})
		args := make([]any, 0, len(sources)*2)
		for _, source := range sources {
			if source.AccountID == "" || source.AccountID == domain.LocalAccountID {
				continue
			}
			args = append(args, source.AccountID, source.Directory.ID)
		}
		selector.OrderExpr(sql.Expr(query, args...))
	}
}

func libraryEntity(id, name *string) *Entity {
	if name == nil || *name == "" {
		return nil
	}
	return &Entity{ID: domain.ValueOrZero(id), Name: *name}
}
