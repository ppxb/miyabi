package offline

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
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
	records, err := latestOfflineTasks(ctx, service.database.Task.Query().Where(task.TypeEQ(tasks.KindOffline.String()), func(s *sql.Selector) {
		s.Where(sql.And(
			sqljson.ValueEQ(task.FieldPayload, source.AccountID, sqljson.Path("account_id")),
			sqljson.ValueEQ(task.FieldPayload, source.Directory.ID, sqljson.Path("directory_id")),
		))
	}))
	if err != nil {
		return result, fmt.Errorf("load offline activity: %w", err)
	}
	result.Tasks, err = service.submissions(ctx, records, source)
	return result, err
}

// Tasks projects history through the current file index and workflow. A
// finished remote task alone never means the resource still exists.
func (service *Service) Tasks(ctx context.Context, movieID, accountID string) ([]domain.OfflineSubmission, error) {
	records, err := latestOfflineTasks(ctx, service.database.Task.Query().Where(task.TypeEQ(tasks.KindOffline.String()), func(s *sql.Selector) {
		s.Where(sql.And(
			sqljson.ValueEQ(task.FieldPayload, movieID, sqljson.Path("javdb_id")),
			sqljson.ValueEQ(task.FieldPayload, accountID, sqljson.Path("account_id")),
		))
	}))
	if err != nil {
		return nil, fmt.Errorf("load movie offline tasks: %w", err)
	}
	source := service.drive.Source()
	return service.submissions(ctx, records, source)
}

func (service *Service) submission(ctx context.Context, record *ent.Task, source *domain.LibrarySource) (domain.OfflineSubmission, error) {
	items, err := service.submissions(ctx, []*ent.Task{record}, source)
	if err != nil {
		return domain.OfflineSubmission{}, err
	}
	return items[0], nil
}

// Project task workflows and file presence in batches. The global observer and
// movie buttons share this view without a database query for every download.
func (service *Service) submissions(ctx context.Context, records []*ent.Task, source *domain.LibrarySource) ([]domain.OfflineSubmission, error) {
	inputs := make([]offlinePayload, len(records))
	var scanIDs []int
	var fileIDs []string
	for index, record := range records {
		input, err := tasks.DecodePayload[offlinePayload](record.Payload)
		if err != nil {
			return nil, err
		}
		if input.Hash == "" {
			return nil, fmt.Errorf("offline task %d is missing magnet hash", record.ID)
		}
		inputs[index] = input
		if source == nil || source.AccountID != input.AccountID || source.Directory.ID != input.DirectoryID ||
			record.Status == task.StatusQueued || record.Status == task.StatusRunning {
			continue
		}
		if input.ScanTaskID != 0 {
			scanIDs = append(scanIDs, input.ScanTaskID)
		}
		fileIDs = append(fileIDs, input.FileIDs...)
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
		input := inputs[index]
		item := &result[index]
		*item = domain.OfflineSubmission{
			TaskID:      record.ID,
			Code:        input.Code,
			JavDBID:     input.JavDBID,
			AccountID:   input.AccountID,
			DirectoryID: input.DirectoryID,
			ScanTaskID:  input.ScanTaskID,
			Hash:        input.Hash,
			Status:      string(record.Status),
			Progress:    record.Progress,
			Error:       record.Error,
			Phase:       "available",
		}
		if source == nil || source.AccountID != input.AccountID || source.Directory.ID != input.DirectoryID {
			continue
		}
		if record.Status == task.StatusQueued || record.Status == task.StatusRunning {
			item.Phase = "downloading"
			continue
		}
		if input.ScanTaskID != 0 {
			scan, found := scans[input.ScanTaskID]
			if !found {
				return nil, fmt.Errorf("scan task %d for download %d was not found", input.ScanTaskID, record.ID)
			}
			if scan.Status == string(task.StatusQueued) || scan.Status == string(task.StatusRunning) {
				item.Processing = true
			}
			if scan.Error != nil {
				item.Error = scan.Error
			}
		} else if record.Status == task.StatusDone && (input.FileID != "" || input.AwaitingLocation) {
			// Remote completion may arrive before its file location. Keep the
			// indexing workflow active without presenting it as a download.
			item.Processing = true
		}
		if item.Processing {
			item.Phase = "processing"
		}
		for _, id := range input.FileIDs {
			matched, present := indexed[id]
			if !present {
				continue
			}
			if matched != nil && (matched.JavdbID != nil && *matched.JavdbID == input.JavDBID ||
				matched.JavdbID == nil && matched.Code == codeid.Normalize(input.Code)) {
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

func latestOfflineTasks(ctx context.Context, query *ent.TaskQuery) ([]*ent.Task, error) {
	// Apply the same account/movie/directory scope before grouping. No history
	// limit: a long-running download may be older than every completed task.
	return query.Where(func(s *sql.Selector) {
		payload := s.C(task.FieldPayload)
		// A malformed hash must remain visible to validation even if its JSON
		// representation matches a newer string hash.
		latest := s.Clone().Select(sql.Max(s.C(task.FieldID))).
			GroupBy("json_type("+payload+", '$.hash')", tasks.JSONExtract(payload, "hash"))
		s.Where(sql.In(s.C(task.FieldID), latest))
	}).Order(ent.Desc(task.FieldID)).All(ctx)
}
