package library

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/predicate"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/library/scan"
	"github.com/ppxb/miyabi/internal/tasks"
)

func (s *Service) RescrapeMovie(ctx context.Context, id int, code string) (domain.TaskInfo, error) {
	parent, err := s.scanner.RescrapeMovie(ctx, id, code)
	if err != nil {
		return domain.TaskInfo{}, err
	}
	infos, err := s.Workflows(ctx, []*ent.Task{parent})
	if err != nil {
		return domain.TaskInfo{}, err
	}
	return infos[0], nil
}

// rescrapeFailedBatchSize bounds one failed-movie page so a whole-library retry
// does not block task claiming or the database writer for long.
const rescrapeFailedBatchSize = 500

// RescrapeFailed queues a fresh metadata workflow for every failed movie that
// maps to an enabled source. Queued movies are reported separately and moved to
// pending; movies whose files no longer belong to a current source are skipped.
func (s *Service) RescrapeFailed(ctx context.Context) (created, reused, skipped int, err error) {
	scope := s.scope(ctx)
	mounts, err := s.rescrapeMounts(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	roots, err := s.rescrapeRoots(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	parentID := 0
	for after := 0; ; {
		next, pageCreated, pageReused, pageSkipped, err := s.rescrapeFailedPage(ctx, after, scope, mounts, roots, &parentID)
		if err != nil {
			return 0, 0, 0, err
		}
		created += pageCreated
		reused += pageReused
		skipped += pageSkipped
		if next == 0 {
			break
		}
		after = next
	}
	if created+reused > 0 {
		s.tasks.NotifyLibraryChanged()
		s.tasks.WakePool()
	}
	return created, reused, skipped, nil
}

// FailedRescrapeCount reports how many failed movies RescrapeFailed would
// process now, using the same source resolution as the action itself.
func (s *Service) FailedRescrapeCount(ctx context.Context) (int, error) {
	scope := s.scope(ctx)
	mounts, err := s.rescrapeMounts(ctx)
	if err != nil {
		return 0, err
	}
	roots, err := s.rescrapeRoots(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for after := 0; ; {
		records, err := queryFailedMovies(ctx, s.database.Movie.Query(), after, scope)
		if err != nil {
			return 0, fmt.Errorf("load failed movies: %w", err)
		}
		if len(records) == 0 {
			return count, nil
		}
		jobs, _ := rescrapeJobs(records, mounts, roots)
		count += len(jobs)
		after = records[len(records)-1].ID
	}
}

// rescrapeFailedPage enqueues one bounded page under the queue lock, then
// releases it before the next page so workers and writers can make progress.
func (s *Service) rescrapeFailedPage(ctx context.Context, after int, scope predicate.File, mounts map[string]domain.LibrarySource, roots []rescrapeRoot, parentID *int) (next, created, reused, skipped int, err error) {
	if err := s.tasks.Queue().Lock(ctx); err != nil {
		return 0, 0, 0, 0, err
	}
	defer s.tasks.Queue().Unlock()
	pageParent := 0
	err = ent.WithTx(ctx, s.database, func(tx *ent.Tx) error {
		records, err := queryFailedMovies(ctx, tx.Movie.Query(), after, scope)
		if err != nil {
			return fmt.Errorf("load failed movies: %w", err)
		}
		if len(records) == 0 {
			return nil
		}
		next = records[len(records)-1].ID
		jobs, pageSkipped := rescrapeJobs(records, mounts, roots)
		skipped = pageSkipped
		if len(jobs) == 0 {
			return nil
		}
		plan, err := scan.PlanScrapes(ctx, tx, jobs, scan.ScanScrapePolicy)
		if err != nil {
			return err
		}
		if len(plan.Create) > 0 {
			if *parentID == 0 {
				parent, err := createRescrapeWorkflow(ctx, tx, jobs[0].Source)
				if err != nil {
					return err
				}
				pageParent = parent.ID
			} else {
				pageParent = *parentID
			}
			for i := range plan.Create {
				plan.Create[i].ScanTaskID = pageParent
			}
		}
		if err := scan.CreateScrapeTasks(ctx, tx, plan.Create); err != nil {
			return err
		}
		pending := make([]int, 0, len(plan.Create)+len(plan.Reused))
		for _, job := range plan.Create {
			pending = append(pending, job.MovieID)
		}
		for id := range plan.Reused {
			pending = append(pending, id)
		}
		if len(pending) > 0 {
			if err := tx.Movie.Update().
				Where(movie.IDIn(pending...), movie.ScrapeStatusEQ(movie.ScrapeStatusFailed)).
				SetScrapeStatus(movie.ScrapeStatusPending).Exec(ctx); err != nil {
				return err
			}
		}
		created = len(plan.Create)
		reused = len(plan.Reused)
		return nil
	})
	if err == nil && pageParent != 0 {
		*parentID = pageParent
	}
	return next, created, reused, skipped, err
}

// queryFailedMovies loads one page of failed movies with the files needed to
// resolve their source.
func queryFailedMovies(ctx context.Context, query *ent.MovieQuery, after int, scope predicate.File) ([]*ent.Movie, error) {
	return query.
		Where(movie.IDGT(after), movie.ScrapeStatusEQ(movie.ScrapeStatusFailed), movie.HasFilesWith(scope)).
		WithFiles(func(q *ent.FileQuery) {
			q.Where(scope).Select(file.FieldAccountID, file.FieldPath)
		}).
		Select(movie.FieldID, movie.FieldCode, movie.FieldManualCode).
		Order(movie.ByID()).Limit(rescrapeFailedBatchSize).
		All(ctx)
}

// rescrapeJobs maps failed movies to scrape jobs and counts the movies that
// cannot be mapped to a current source.
func rescrapeJobs(records []*ent.Movie, mounts map[string]domain.LibrarySource, roots []rescrapeRoot) ([]scan.ScrapeJob, int) {
	jobs := make([]scan.ScrapeJob, 0, len(records))
	skipped := 0
	for _, record := range records {
		source, ok := rescrapeSource(record.Edges.Files, mounts, roots)
		if !ok {
			skipped++
			continue
		}
		jobs = append(jobs, scan.ScrapeJob{
			MovieID: record.ID, Code: record.Code, ManualCode: record.ManualCode, Source: source,
		})
	}
	return jobs, skipped
}

// createRescrapeWorkflow creates the parent scan task that makes the queued
// scrape jobs visible, cancellable and retryable in the task list.
func createRescrapeWorkflow(ctx context.Context, tx *ent.Tx, source domain.LibrarySource) (*ent.Task, error) {
	body, err := tasks.EncodePayload(domain.ScanPayload{
		BulkRescrape: true,
		Source:       source,
		Scan:         domain.ScanProgress{Stage: "done", CurrentPath: source.Directory.Path},
	})
	if err != nil {
		return nil, err
	}
	parent, err := tx.Task.Create().SetType(tasks.KindScan.String()).SetStatus(task.StatusDone).SetPayload(body).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create bulk rescrape workflow: %w", err)
	}
	return parent, nil
}

// rescrapeMounts resolves each 115 account once instead of opening a session
// per movie. Accounts without a current mount are left out, so their movies are
// reported as skipped.
func (s *Service) rescrapeMounts(ctx context.Context) (map[string]domain.LibrarySource, error) {
	accounts, err := s.database.File.Query().
		Where(file.AccountIDNEQ(domain.LocalAccountID),
			file.HasMovieWith(movie.ScrapeStatusEQ(movie.ScrapeStatusFailed))).
		Select(file.FieldAccountID).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list failed movie accounts: %w", err)
	}
	mounts := make(map[string]domain.LibrarySource)
	for _, entry := range accounts {
		if _, ok := mounts[entry.AccountID]; ok {
			continue
		}
		source, ok, err := s.drive.Mount(ctx, entry.AccountID)
		if err != nil {
			return nil, fmt.Errorf("resolve mount for account %s: %w", entry.AccountID, err)
		}
		if ok {
			mounts[entry.AccountID] = source
		}
	}
	return mounts, nil
}

// rescrapeRoot is one local directory a failed movie may belong to.
type rescrapeRoot struct {
	path         string
	name         string
	metadataOnly bool
}

func (s *Service) rescrapeRoots(ctx context.Context) ([]rescrapeRoot, error) {
	var roots []rescrapeRoot
	if s.localSources != nil {
		targets, err := s.localSources(ctx)
		if err != nil {
			return nil, fmt.Errorf("list local sources: %w", err)
		}
		for _, target := range targets {
			path, err := canonicalDirectory(target.Path)
			if err != nil {
				continue
			}
			roots = append(roots, rescrapeRoot{path: path, name: "本地目录", metadataOnly: target.MetadataOnly})
		}
	}
	if s.exportMgr != nil {
		if dir := s.exportMgr.Config().EmbyDir; dir != "" {
			if path, err := canonicalDirectory(dir); err == nil {
				roots = append(roots, rescrapeRoot{path: path, name: embyLocalDirectoryName})
			}
		}
	}
	return roots, nil
}

// rescrapeSource maps a movie's files to the same source a scan would use:
// the first mounted 115 account, otherwise the configured local root that
// contains the file. Local sources in metadata-only mode are not scraped.
func rescrapeSource(files []*ent.File, mounts map[string]domain.LibrarySource, roots []rescrapeRoot) (domain.LibrarySource, bool) {
	for _, entry := range files {
		if entry.AccountID == domain.LocalAccountID {
			continue
		}
		if source, ok := mounts[entry.AccountID]; ok {
			return source, true
		}
	}
	for _, entry := range files {
		if entry.AccountID != domain.LocalAccountID || entry.Path == "" {
			continue
		}
		for _, root := range roots {
			if root.metadataOnly || !underDirectory(root.path, entry.Path) {
				continue
			}
			return domain.LibrarySource{
				AccountID: domain.LocalAccountID,
				Directory: domain.LibraryDirectory{ID: root.path, Name: root.name, Path: root.path},
			}, true
		}
	}
	return domain.LibrarySource{}, false
}

func underDirectory(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
