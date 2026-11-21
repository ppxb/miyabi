package library

import (
	"context"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	subtitlemeta "github.com/ppxb/miyabi/internal/domain/subtitle"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/predicate"
	"github.com/ppxb/miyabi/internal/ent/subtitle"
	"github.com/ppxb/miyabi/internal/ent/tag"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/ent/watchhistory"
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

const finishedProgressRatio = 0.95

type Movie struct {
	ID           int                `json:"id"`
	Code         string             `json:"code"`
	Title        string             `json:"title"`
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
	// HasSubtitle marks movies whose file or indexed subtitle names indicate a
	// Chinese-subtitle release.
	HasSubtitle bool `json:"has_subtitle"`
	// Finished marks movies whose saved playback position reached the end.
	Finished bool `json:"finished,omitempty"`
}

type Page struct {
	Source  *domain.LibrarySource `json:"source,omitempty"`
	Movies  []Movie               `json:"movies"`
	Total   int                   `json:"total"`
	Page    int                   `json:"page"`
	HasMore bool                  `json:"has_more"`
}

// BrowseFilter controls a filtered, paginated library browse.
type BrowseFilter struct {
	Query       string
	Sort        string // "recent" (default), "rating", "release"
	Source      string // "" (all), "115", "local"
	HighRated   bool
	TagNames    []string
	ActorNames  []string
	MakerNames  []string
	SeriesNames []string
	Page        int
	Limit       int
}

// FilterOption is one selectable value for a browse filter.
type FilterOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// FilterOptions lists the values available for browsing filters.
type FilterOptions struct {
	Tags   []FilterOption `json:"tags"`
	Actors []FilterOption `json:"actors"`
	Makers []FilterOption `json:"makers"`
	Series []FilterOption `json:"series"`
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
	info, queued, err := s.enqueueAllScans(ctx, false)
	if err != nil {
		return domain.TaskInfo{}, err
	}
	if !queued {
		return domain.TaskInfo{}, domain.E(domain.KindInvalid, "没有可扫描的存储源，请先在设置 → 源管理中添加", nil)
	}
	return info, nil
}

// StartRebuild scans the current source and refreshes every matched movie.
func (s *Service) StartRebuild(ctx context.Context) (domain.TaskInfo, error) {
	info, queued, err := s.enqueueAllScans(ctx, true)
	if err != nil {
		return domain.TaskInfo{}, err
	}
	if !queued {
		return domain.TaskInfo{}, domain.E(domain.KindInvalid, "没有可扫描的存储源，请先在设置 → 源管理中添加", nil)
	}
	return info, nil
}

// enqueueAllScans queues a full scan (or rebuild) for every enabled source:
// configured local directories and each mounted 115 account, plus the current
// primary mount. It reports the first queued task and whether anything was
// configured, so callers can fall back to the single primary mount only when no
// sources exist. This keeps multi-account and local-only setups from needlessly
// requiring one specific 115 account to be logged in.
func (s *Service) enqueueAllScans(ctx context.Context, rebuild bool) (domain.TaskInfo, bool, error) {
	var first domain.TaskInfo
	queued := false
	record := func(info domain.TaskInfo, err error) error {
		if err != nil {
			return err
		}
		if !queued {
			first, queued = info, true
		}
		return nil
	}

	if s.localSources != nil {
		targets, err := s.localSources(ctx)
		if err != nil {
			return first, queued, err
		}
		for _, target := range targets {
			info, err := s.startLocalSourceScan(ctx, target.Path, target.MetadataOnly, false)
			if err := record(info, err); err != nil {
				return first, queued, err
			}
		}
	}

	var cloud []domain.LibrarySource
	if s.drive != nil {
		mounts, err := s.drive.Mounts(ctx)
		if err != nil {
			return first, queued, err
		}
		cloud = mounts
	}
	for _, src := range cloud {
		var info domain.TaskInfo
		var err error
		if rebuild {
			info, err = s.enqueueScan(ctx, src, true, task.StatusQueued, task.StatusRunning)
		} else {
			info, err = s.EnqueueScan(ctx, src)
		}
		if err := record(info, err); err != nil {
			return first, queued, err
		}
	}
	return first, queued, nil
}

// MatchingMovies queries local movies matching any of the given JavDB IDs or normalized codes
// that possess indexed files in the currently mounted library source or local library.
func (s *Service) MatchingMovies(ctx context.Context, codes []string) ([]domain.LocalMovie, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	records, err := s.database.Movie.Query().Where(
		movie.CodeIn(codes...),
		movie.HasFilesWith(s.scope(ctx)),
	).Select(movie.FieldID, movie.FieldCode).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query matching movies: %w", err)
	}
	result := make([]domain.LocalMovie, len(records))
	for i, r := range records {
		result[i] = domain.LocalMovie{
			ID:   r.ID,
			Code: r.Code,
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
func (s *Service) EnqueueTargetedScan(ctx context.Context, tx *ent.Tx, source domain.LibrarySource, targetID string, offlineTaskID int, code string) (int, error) {
	encoded, err := tasks.EncodePayload(domain.ScanPayload{
		Source:        source,
		Scan:          domain.ScanProgress{Stage: "queued", CurrentPath: source.Directory.Path},
		TargetID:      targetID,
		OfflineTaskID: offlineTaskID,
		Code:          code,
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

// cloudSources aggregates every mounted 115 account plus configured sources so
// the library lists movies across all accounts, not just one primary mount.
func (s *Service) cloudSources(ctx context.Context) []domain.LibrarySource {
	return s.mergeSources(ctx, s.mountList(ctx))
}

// mountList loads every mounted 115 account's library source once, tolerating
// errors like the aggregate helpers do.
func (s *Service) mountList(ctx context.Context) []domain.LibrarySource {
	if s.drive == nil {
		return nil
	}
	mounts, err := s.drive.Mounts(ctx)
	if err != nil {
		return nil
	}
	return mounts
}

// mergeSources combines already-loaded mounted sources with configured sources,
// de-duplicated by account and directory.
func (s *Service) mergeSources(ctx context.Context, mounts []domain.LibrarySource) []domain.LibrarySource {
	seen := make(map[string]bool)
	result := make([]domain.LibrarySource, 0, len(mounts))
	add := func(src domain.LibrarySource) {
		if src.Directory.ID == "" {
			return
		}
		key := src.AccountID + "\x00" + src.Directory.ID
		if seen[key] {
			return
		}
		seen[key] = true
		result = append(result, src)
	}
	for _, mount := range mounts {
		add(mount)
	}
	if s.sources != nil {
		if list, err := s.sources(ctx); err == nil {
			for _, source := range list {
				add(source)
			}
		}
	}
	return result
}

// firstSource returns the first mounted source for display, or nil.
func firstSource(mounts []domain.LibrarySource) *domain.LibrarySource {
	if len(mounts) == 0 {
		return nil
	}
	return &mounts[0]
}

// Source returns one mounted library source (the first by account id). It
// satisfies catalogue.LocalState; request paths load mounts once via mountList.
func (s *Service) Source(ctx context.Context) *domain.LibrarySource {
	return firstSource(s.mountList(ctx))
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
	mounts := s.mountList(ctx)
	sources := s.mergeSources(ctx, mounts)
	result := Page{Movies: []Movie{}, Page: page, Source: firstSource(mounts)}
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
		Select(movie.FieldID, movie.FieldCode, movie.FieldTitle, movie.FieldCover, movie.FieldPoster,
			movie.FieldFanarts, movie.FieldReleaseDate, movie.FieldDuration, movie.FieldRating,
			movie.FieldRating10,
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
		result.Movies = append(result.Movies, movieItem(record))
	}
	if err := s.markCloudSources(ctx, result.Movies, cloudScope); err != nil {
		return result, err
	}
	if err := s.markSubtitleStatus(ctx, result.Movies); err != nil {
		return result, err
	}
	if err := s.markWatchState(ctx, result.Movies); err != nil {
		return result, err
	}
	result.HasMore = (page-1)*limit+len(result.Movies) < result.Total
	return result, nil
}

func movieItem(record *ent.Movie) Movie {
	item := Movie{
		ID: record.ID, Code: record.Code, Title: record.Title,
		Cover: record.Cover, Poster: record.Poster,
		Duration: domain.ValueOrZero(record.Duration),
		Rating:   record.Rating10,
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
	return item
}

// TopRated lists the highest rated movies on a normalized 10-point scale.
func (s *Service) TopRated(ctx context.Context, limit int) ([]Movie, error) {
	if limit <= 0 {
		limit = 12
	}
	if limit > 50 {
		limit = 50
	}
	sources := s.cloudSources(ctx)
	scope := fileScope(sources)
	cloudScope := cloudFileScope(sources)
	records, err := s.database.Movie.Query().
		Where(movie.HasFilesWith(scope), movie.RatingNotNil(), movie.RatingGT(0)).
		Select(browseScalarFields...).
		Order(ent.Desc(movie.FieldRating10), ent.Desc(movie.FieldID)).
		Limit(limit).
		WithActors(func(query *ent.ActorQuery) {
			query.Select(actor.FieldID, actor.FieldProvider, actor.FieldSourceID, actor.FieldName).Order(ent.Asc(actor.FieldName), ent.Asc(actor.FieldID))
		}).
		WithTags(func(query *ent.TagQuery) {
			query.Select(tag.FieldID, tag.FieldProvider, tag.FieldSourceID, tag.FieldName).Order(ent.Asc(tag.FieldName), ent.Asc(tag.FieldID))
		}).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list top rated movies: %w", err)
	}
	items := make([]Movie, 0, len(records))
	for _, record := range records {
		items = append(items, movieItem(record))
	}
	if err := s.markCloudSources(ctx, items, cloudScope); err != nil {
		return nil, err
	}
	if err := s.markSubtitleStatus(ctx, items); err != nil {
		return nil, err
	}
	if err := s.markWatchState(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
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

// markSubtitleStatus flags movies whose file or indexed subtitle names carry a
// Chinese-subtitle marker, keeping list cards consistent with the detail view.
func (s *Service) markSubtitleStatus(ctx context.Context, movies []Movie) error {
	if len(movies) == 0 {
		return nil
	}
	ids := make([]int, len(movies))
	index := make(map[int]*Movie, len(movies))
	for i := range movies {
		ids[i] = movies[i].ID
		index[movies[i].ID] = &movies[i]
	}
	files, err := s.database.File.Query().
		Where(file.MovieIDIn(ids...)).
		Select(file.FieldMovieID, file.FieldName).All(ctx)
	if err != nil {
		return fmt.Errorf("read library subtitle files: %w", err)
	}
	for _, record := range files {
		if record.MovieID != nil && subtitlemeta.HasChineseSubtitle(record.Name) {
			if item, ok := index[*record.MovieID]; ok {
				item.HasSubtitle = true
			}
		}
	}
	tracks, err := s.database.Subtitle.Query().
		Where(subtitle.MovieIDIn(ids...)).
		Select(subtitle.FieldMovieID, subtitle.FieldName).All(ctx)
	if err != nil {
		return fmt.Errorf("read library subtitles: %w", err)
	}
	for _, track := range tracks {
		if subtitlemeta.HasChineseSubtitle(track.Name) {
			if item, ok := index[track.MovieID]; ok {
				item.HasSubtitle = true
			}
		}
	}
	return nil
}

// markWatchState flags movies whose saved playback position reached the end,
// matching the resume filter used by the "continue watching" shelf.
func (s *Service) markWatchState(ctx context.Context, movies []Movie) error {
	if len(movies) == 0 {
		return nil
	}
	ids := make([]int, len(movies))
	index := make(map[int]*Movie, len(movies))
	for i := range movies {
		ids[i] = movies[i].ID
		index[movies[i].ID] = &movies[i]
	}
	records, err := s.database.WatchHistory.Query().
		Where(watchhistory.MovieIDIn(ids...)).
		Select(watchhistory.FieldMovieID, watchhistory.FieldPosition, watchhistory.FieldDuration).All(ctx)
	if err != nil {
		return fmt.Errorf("read library watch history: %w", err)
	}
	for _, record := range records {
		if record.Duration > 0 && record.Position/record.Duration >= finishedProgressRatio {
			if item, ok := index[record.MovieID]; ok {
				item.Finished = true
			}
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

// highRatedThreshold is the normalized (10-point) rating floor for "high rated".
const highRatedThreshold = 8.0

// browseScalarFields are the columns required to build a Movie without edges.
var browseScalarFields = []string{
	movie.FieldID, movie.FieldCode, movie.FieldTitle,
	movie.FieldCover, movie.FieldPoster, movie.FieldFanarts, movie.FieldReleaseDate,
	movie.FieldDuration, movie.FieldRating, movie.FieldRating10,
	movie.FieldDirectorID, movie.FieldDirectorName, movie.FieldMakerID, movie.FieldMakerName,
	movie.FieldSeriesID, movie.FieldSeriesName, movie.FieldScrapeStatus,
}

func normalizeSort(value string) string {
	switch value {
	case "rating", "release", "recent":
		return value
	default:
		return "recent"
	}
}

func normalizeSource(value string) string {
	switch value {
	case "115", "local":
		return value
	default:
		return ""
	}
}

// BrowseMovies lists library movies filtered by keyword, source, rating, tags,
// actors and makers. Filters, sorting and pagination are computed over the
// matching rows; only the current page's actors/tags are preloaded.
func (s *Service) BrowseMovies(ctx context.Context, filter BrowseFilter) (Page, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	filter.Sort = normalizeSort(filter.Sort)
	filter.Source = normalizeSource(filter.Source)
	mounts := s.mountList(ctx)
	sources := s.mergeSources(ctx, mounts)
	result := Page{Movies: []Movie{}, Page: filter.Page, Source: firstSource(mounts)}
	scope := fileScope(sources)
	cloudScope := cloudFileScope(sources)

	preds := []predicate.Movie{movie.HasFilesWith(scope)}
	if q := strings.TrimSpace(filter.Query); q != "" {
		preds = append(preds, movie.Or(
			movie.CodeContainsFold(q),
			movie.TitleContainsFold(q),
			movie.HasActorsWith(actor.NameContainsFold(q)),
		))
	}
	switch filter.Source {
	case "local":
		preds = append(preds, movie.HasFilesWith(file.AccountIDEQ(domain.LocalAccountID)))
	case "115":
		if cloudScope == nil {
			return result, nil
		}
		preds = append(preds, movie.HasFilesWith(cloudScope))
	}
	if names := nonEmptyStrings(filter.TagNames); len(names) > 0 {
		matched, err := s.database.Tag.Query().Where(tag.NameIn(names...)).Select(tag.FieldID).All(ctx)
		if err != nil {
			return result, fmt.Errorf("resolve tag filters: %w", err)
		}
		if len(matched) == 0 {
			return result, nil
		}
		ids := make([]int, len(matched))
		for i, item := range matched {
			ids[i] = item.ID
		}
		preds = append(preds, movie.HasTagsWith(tag.IDIn(ids...)))
	}
	if names := nonEmptyStrings(filter.ActorNames); len(names) > 0 {
		matched, err := s.database.Actor.Query().Where(actor.NameIn(names...)).Select(actor.FieldID).All(ctx)
		if err != nil {
			return result, fmt.Errorf("resolve actor filters: %w", err)
		}
		if len(matched) == 0 {
			return result, nil
		}
		ids := make([]int, len(matched))
		for i, item := range matched {
			ids[i] = item.ID
		}
		preds = append(preds, movie.HasActorsWith(actor.IDIn(ids...)))
	}
	if names := nonEmptyStrings(filter.MakerNames); len(names) > 0 {
		preds = append(preds, movie.MakerNameIn(names...))
	}
	if names := nonEmptyStrings(filter.SeriesNames); len(names) > 0 {
		preds = append(preds, movie.SeriesNameIn(names...))
	}

	if filter.HighRated {
		preds = append(preds, movie.Rating10GTE(highRatedThreshold))
	}

	total, err := s.database.Movie.Query().Where(preds...).Count(ctx)
	if err != nil {
		return result, fmt.Errorf("count library movies: %w", err)
	}
	result.Total = total
	lastPage := (total + filter.Limit - 1) / filter.Limit
	if lastPage < 1 {
		lastPage = 1
	}
	if filter.Page > lastPage {
		filter.Page = lastPage
	}
	result.Page = filter.Page
	start := (filter.Page - 1) * filter.Limit

	query := s.database.Movie.Query().Where(preds...).
		Select(browseScalarFields...).
		WithActors(func(query *ent.ActorQuery) {
			query.Select(actor.FieldID, actor.FieldProvider, actor.FieldSourceID, actor.FieldName).Order(ent.Asc(actor.FieldName), ent.Asc(actor.FieldID))
		}).
		WithTags(func(query *ent.TagQuery) {
			query.Select(tag.FieldID, tag.FieldProvider, tag.FieldSourceID, tag.FieldName).Order(ent.Asc(tag.FieldName), ent.Asc(tag.FieldID))
		})
	switch filter.Sort {
	case "rating":
		query = query.Order(ent.Desc(movie.FieldRating10), ent.Desc(movie.FieldID))
	case "release":
		query = query.Order(ent.Desc(movie.FieldReleaseDate), ent.Desc(movie.FieldID))
	default:
		order := []movie.OrderOption{ent.Desc(movie.FieldCreatedAt), ent.Desc(movie.FieldID)}
		if cloudScope != nil {
			order = append([]movie.OrderOption{cloudAddedOrder(sources)}, order...)
		}
		query = query.Order(order...)
	}
	records, err := query.Offset(start).Limit(filter.Limit).All(ctx)
	if err != nil {
		return result, fmt.Errorf("browse library movies: %w", err)
	}

	items := make([]Movie, 0, len(records))
	for _, record := range records {
		items = append(items, movieItem(record))
	}
	result.Movies = items
	if err := s.markCloudSources(ctx, result.Movies, cloudScope); err != nil {
		return result, err
	}
	if err := s.markSubtitleStatus(ctx, result.Movies); err != nil {
		return result, err
	}
	if err := s.markWatchState(ctx, result.Movies); err != nil {
		return result, err
	}
	result.HasMore = start+len(items) < total
	return result, nil
}

// FilterOptions lists the tags, actors, makers and series present in the library
// scope so the browse page can offer deduplicated filters.
func (s *Service) FilterOptions(ctx context.Context) (FilterOptions, error) {
	result := FilterOptions{
		Tags:   []FilterOption{},
		Actors: []FilterOption{},
		Makers: []FilterOption{},
		Series: []FilterOption{},
	}
	scope := fileScope(s.cloudSources(ctx))
	movieInScope := movie.HasFilesWith(scope)

	tags, err := s.database.Tag.Query().
		Where(tag.HasMoviesWith(movieInScope)).
		Order(ent.Asc(tag.FieldName), ent.Asc(tag.FieldID)).
		All(ctx)
	if err != nil {
		return result, fmt.Errorf("list filter tags: %w", err)
	}
	seen := make(map[string]struct{}, len(tags))
	for _, item := range tags {
		if _, ok := seen[item.Name]; ok {
			continue
		}
		seen[item.Name] = struct{}{}
		result.Tags = append(result.Tags, FilterOption{ID: item.Name, Name: item.Name})
	}

	actors, err := s.database.Actor.Query().
		Where(actor.HasMoviesWith(movieInScope)).
		Order(ent.Asc(actor.FieldName), ent.Asc(actor.FieldID)).
		All(ctx)
	if err != nil {
		return result, fmt.Errorf("list filter actors: %w", err)
	}
	seen = make(map[string]struct{}, len(actors))
	for _, item := range actors {
		if _, ok := seen[item.Name]; ok {
			continue
		}
		seen[item.Name] = struct{}{}
		result.Actors = append(result.Actors, FilterOption{ID: item.Name, Name: item.Name})
	}

	result.Makers = s.distinctMovieNames(ctx, movieInScope, movie.FieldMakerName, movie.MakerNameNotNil(), movie.MakerNameNEQ(""))
	result.Series = s.distinctMovieNames(ctx, movieInScope, movie.FieldSeriesName, movie.SeriesNameNotNil(), movie.SeriesNameNEQ(""))
	return result, nil
}

// distinctMovieNames collects the distinct non-empty values of a movie name
// column (maker/series) within the library scope, sorted for stable display.
func (s *Service) distinctMovieNames(ctx context.Context, scope predicate.Movie, field string, extra ...predicate.Movie) []FilterOption {
	preds := append([]predicate.Movie{scope}, extra...)
	records, err := s.database.Movie.Query().
		Where(preds...).
		Order(ent.Asc(field)).
		Select(field).
		All(ctx)
	if err != nil {
		return []FilterOption{}
	}
	result := make([]FilterOption, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		name := movieNameField(record, field)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, FilterOption{ID: name, Name: name})
	}
	return result
}

func movieNameField(record *ent.Movie, field string) string {
	switch field {
	case movie.FieldMakerName:
		if record.MakerName != nil {
			return *record.MakerName
		}
	case movie.FieldSeriesName:
		if record.SeriesName != nil {
			return *record.SeriesName
		}
	}
	return ""
}

func nonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
