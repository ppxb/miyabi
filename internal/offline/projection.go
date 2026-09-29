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
)

// Activity summarizes current offline download tasks alongside the active library source.
type Activity struct {
	Source *domain.LibrarySource      `json:"source,omitempty"`
	Tasks  []domain.OfflineSubmission `json:"tasks"`
}

// Activity only reads local tasks and the mounted file index. Keeping each
// magnet's latest workflow also retains long downloads until their final state.
func (service *Service) Activity(ctx context.Context) (Activity, error) {
	result := Activity{Tasks: []domain.OfflineSubmission{}}
	source := service.drive.Source()
	result.Source = source
	if source == nil {
		return result, nil
	}
	records, err := latestOfflineTasks(ctx, service.database.OfflineDownload.Query().Where(
		offlinedownload.AccountIDEQ(source.AccountID), offlinedownload.DirectoryIDEQ(source.Directory.ID)))
	if err != nil {
		return result, fmt.Errorf("load offline activity: %w", err)
	}
	result.Tasks, err = service.submissions(ctx, records, source)
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
		if record.Hash == "" {
			return nil, fmt.Errorf("offline task %d is missing magnet hash", record.ID)
		}
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
				query.Select(movie.FieldID, movie.FieldCode, movie.FieldJavdbID, movie.FieldScrapeStatus)
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
			JavDBID:     record.JavdbID,
			AccountID:   record.AccountID,
			DirectoryID: record.DirectoryID,
			ScanTaskID:  record.ScanTaskID,
			Hash:        record.Hash,
			Status:      string(record.Status),
			Progress:    record.Progress,
			Error:       record.Error,
			Phase:       "available",
		}
		if source == nil || source.AccountID != record.AccountID || source.Directory.ID != record.DirectoryID {
			continue
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
			if scan.Error != nil {
				item.Error = scan.Error
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
			if matched != nil && (matched.JavdbID != nil && *matched.JavdbID == record.JavdbID ||
				matched.JavdbID == nil && matched.Code == codeid.Normalize(record.Code)) {
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
