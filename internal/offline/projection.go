package offline

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/offlinedownload"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/pan"
)

// Activity summarizes current offline download tasks alongside the active library source.
type Activity struct {
	Source *domain.LibrarySource      `json:"source,omitempty"`
	Tasks  []domain.OfflineSubmission `json:"tasks"`
}

// Activity only reads local tasks and the mounted file index. Keeping each
// magnet's latest workflow also retains long downloads until their final state.
// Activity summarizes offline tasks. With a single mounted account it reports
// that account (backwards compatible); with several it aggregates all of them.
func (service *Service) Activity(ctx context.Context) (Activity, error) {
	sources, err := service.drive.Mounts(ctx)
	if err != nil {
		return Activity{}, err
	}
	if len(sources) == 0 {
		return Activity{Tasks: []domain.OfflineSubmission{}}, nil
	}
	if len(sources) == 1 {
		return service.activityFor(ctx, sources[0])
	}
	result := Activity{Tasks: []domain.OfflineSubmission{}}
	for _, source := range sources {
		activity, err := service.activityFor(ctx, source)
		if err != nil {
			return Activity{}, err
		}
		result.Tasks = append(result.Tasks, activity.Tasks...)
	}
	return result, nil
}

// ActivityForAccount summarizes one account's offline tasks (or every account
// when accountID is empty).
func (service *Service) ActivityForAccount(ctx context.Context, accountID string) (Activity, error) {
	if accountID == "" {
		return service.Activity(ctx)
	}
	source, ok, err := service.drive.Mount(ctx, accountID)
	if err != nil {
		return Activity{}, err
	}
	if !ok {
		return Activity{Tasks: []domain.OfflineSubmission{}}, nil
	}
	return service.activityFor(ctx, source)
}

func (service *Service) activityFor(ctx context.Context, source domain.LibrarySource) (Activity, error) {
	result := Activity{Source: &source, Tasks: []domain.OfflineSubmission{}}
	records, err := latestOfflineTasks(ctx, service.database.OfflineDownload.Query().Where(
		offlinedownload.AccountIDEQ(source.AccountID), offlinedownload.DirectoryIDEQ(source.Directory.ID)))
	if err != nil {
		return result, fmt.Errorf("load offline activity: %w", err)
	}
	result.Tasks, err = service.submissions(ctx, records, &source)
	return result, err
}

func (service *Service) submission(ctx context.Context, record *ent.OfflineDownload, source *domain.LibrarySource) (domain.OfflineSubmission, error) {
	items, err := service.submissions(ctx, []*ent.OfflineDownload{record}, source)
	if err != nil {
		return domain.OfflineSubmission{}, err
	}
	return items[0], nil
}

// Project task workflows and file presence in batches. The global observer and
// movie buttons share this view without a database query for every download.
func (service *Service) submissions(ctx context.Context, records []*ent.OfflineDownload, source *domain.LibrarySource) ([]domain.OfflineSubmission, error) {
	var scanIDs []int
	var fileIDs []string
	for _, record := range records {
		if source == nil || source.AccountID != record.AccountID || source.Directory.ID != record.DirectoryID ||
			record.Status == offlinedownload.StatusRunning {
			continue
		}
		if record.ScanTaskID != 0 {
			scanIDs = append(scanIDs, record.ScanTaskID)
		}
		fileIDs = append(fileIDs, record.FileIds...)
	}
	scans := make(map[int]domain.TaskInfo)
	for start := 0; start < len(scanIDs); start += 500 {
		parents, err := service.database.Task.Query().Where(task.IDIn(scanIDs[start:min(start+500, len(scanIDs))]...)).All(ctx)
		if err != nil {
			return nil, fmt.Errorf("read download scan tasks: %w", err)
		}
		infos, err := service.library.Workflows(ctx, parents)
		if err != nil {
			return nil, err
		}
		for _, info := range infos {
			scans[info.ID] = info
		}
	}
	indexed := make(map[string]*ent.Movie)
	for start := 0; start < len(fileIDs); start += 500 {
		files, err := service.database.File.Query().Where(
			file.AccountIDEQ(source.AccountID),
			file.RootIDEQ(source.Directory.ID),
			file.FileIDIn(fileIDs[start:min(start+500, len(fileIDs))]...)).
			Select(file.FieldFileID, file.FieldMovieID).
			WithMovie(func(query *ent.MovieQuery) {
				query.Select(movie.FieldID, movie.FieldCode, movie.FieldScrapeStatus)
			}).All(ctx)
		if err != nil {
			return nil, fmt.Errorf("read downloaded file index: %w", err)
		}
		for _, entry := range files {
			indexed[entry.FileID] = entry.Edges.Movie
		}
	}
	result := make([]domain.OfflineSubmission, len(records))
	for index, record := range records {
		item := &result[index]
		*item = domain.OfflineSubmission{
			TaskID:      record.ID,
			Code:        record.Code,
			AccountID:   record.AccountID,
			DirectoryID: record.DirectoryID,
			ScanTaskID:  record.ScanTaskID,
			Hash:        currentHash(record),
			Status:      string(record.Status),
			Progress:    record.Progress,
			Error:       record.Error,
			Phase:       "available",
		}
		if source == nil || source.AccountID != record.AccountID || source.Directory.ID != record.DirectoryID {
			continue
		}
		item.CanCancel = record.Status == offlinedownload.StatusRunning || record.Status == offlinedownload.StatusFailed
		item.CanSwitch = item.CanCancel
		if state := record.Recovery; state != nil {
			item.AttemptCount, item.SwitchReason = len(state.Attempts), state.Reason
			item.CanSwitch = item.CanSwitch && state.Action == ""
			if state.RetryAt.After(service.now()) && item.CanCancel {
				item.RetryAt = &state.RetryAt
			}
			switch {
			case state.Action == actionCancel:
				item.DownloadState = "cancelling"
			case state.Action == actionSwitch:
				item.DownloadState = "switching"
			case state.Action == actionSubmit:
				item.DownloadState = "submitting"
			case state.Exhausted:
				item.DownloadState = "exhausted"
			case state.Stalled:
				item.DownloadState = "stalled"
			case state.RemoteStatus == pan.OfflineStatusWaiting:
				item.DownloadState = "queued"
			}
			if item.RetryAt != nil {
				item.DownloadState = "waiting"
			}
			if !item.CanCancel {
				item.DownloadState = ""
			}
		}
		if record.Status == offlinedownload.StatusRunning {
			item.Phase = "downloading"
			continue
		}
		if record.ScanTaskID != 0 {
			scan, found := scans[record.ScanTaskID]
			if !found {
				return nil, fmt.Errorf("scan task %d for download %d was not found", record.ScanTaskID, record.ID)
			}
			if scan.Status == string(task.StatusQueued) || scan.Status == string(task.StatusRunning) {
				item.Processing = true
			}
		} else if record.Status == offlinedownload.StatusDone && (record.FileID != "" || record.AwaitingLocation) {
			// Remote completion may arrive before its file location. Keep the
			// indexing workflow active without presenting it as a download.
			item.Processing = true
		}
		if item.Processing {
			item.Phase = "processing"
		}
		for _, id := range record.FileIds {
			matched, present := indexed[id]
			if !present {
				continue
			}
			if matched != nil && matched.Code == codeid.Normalize(record.Code) {
				item.Phase = "in_library"
				item.LibraryID = matched.ID
				if matched.ScrapeStatus != movie.ScrapeStatusFailed {
					item.Error = nil
				}
				break
			}
			if !item.Processing {
				item.Phase = "downloaded"
			}
		}
	}
	return result, nil
}

func latestOfflineTasks(ctx context.Context, query *ent.OfflineDownloadQuery) ([]*ent.OfflineDownload, error) {
	// Scope first, then keep each magnet's newest record without a history limit.
	return query.Where(func(s *sql.Selector) {
		latest := s.Clone().Select(sql.Max(s.C(offlinedownload.FieldID))).GroupBy(s.C(offlinedownload.FieldHash))
		s.Where(sql.In(s.C(offlinedownload.FieldID), latest))
	}).Order(ent.Desc(offlinedownload.FieldID)).All(ctx)
}
