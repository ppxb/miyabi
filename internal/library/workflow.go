package library

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

type metadataTaskGroup struct {
	ParentID  int         `json:"parent_id"`
	Count     int         `json:"count"`
	Type      string      `json:"type"`
	Status    task.Status `json:"status"`
	Error     *string     `json:"error"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// ListTasks returns recent and active scans, folded with their metadata jobs.
func (s *Service) ListTasks(ctx context.Context) ([]domain.TaskInfo, error) {
	database := s.database
	records, err := database.Task.Query().Where(task.TypeEQ(string(tasks.KindScan))).
		Order(ent.Desc(task.FieldID)).Limit(20).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list scan tasks: %w", err)
	}
	active, err := database.Task.Query().Where(task.TypeIn(string(tasks.KindScan), string(tasks.KindScrape), string(tasks.KindCover)),
		task.StatusIn(task.StatusQueued, task.StatusRunning), func(s *sql.Selector) {
			s.Select("CASE WHEN type = '" + string(tasks.KindScan) + "' THEN id ELSE " + tasks.JSONExtract(task.FieldPayload, "scan_task_id") + " END").Distinct()
		}).Select(task.FieldID).Ints(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active library tasks: %w", err)
	}
	ids := make(map[int]bool)
	for _, record := range records {
		ids[record.ID] = true
	}
	var missing []int
	for _, id := range active {
		if !ids[id] {
			missing = append(missing, id)
			ids[id] = true
		}
	}
	if len(missing) > 0 {
		parents, err := database.Task.Query().Where(task.IDIn(missing...)).All(ctx)
		if err != nil {
			return nil, err
		}
		records = append(records, parents...)
	}
	return s.Workflows(ctx, records)
}

// Workflows folds metadata child tasks into their parent scan workflow summaries.
func (s *Service) Workflows(ctx context.Context, records []*ent.Task) ([]domain.TaskInfo, error) {
	database := s.database
	result := make([]domain.TaskInfo, 0, len(records))
	if len(records) == 0 {
		return result, nil
	}
	children, err := metadataGroups(ctx, database, records)
	if err != nil {
		return nil, fmt.Errorf("read metadata workflow progress: %w", err)
	}
	byParent := make(map[int][]metadataTaskGroup)
	for _, child := range children {
		byParent[child.ParentID] = append(byParent[child.ParentID], child)
	}
	for _, record := range records {
		info, err := scanTaskInfo(record)
		if err != nil {
			return nil, err
		}
		active, running, failed, artwork := false, false, false, false
		for _, child := range byParent[record.ID] {
			if child.Type == string(tasks.KindScrape) {
				info.Scan.MetadataTotal += child.Count
			}
			if (child.Type == string(tasks.KindCover) && child.Status == task.StatusDone) || child.Status == task.StatusFailed {
				info.Scan.MetadataCompleted += child.Count
			}
			active = active || child.Status == task.StatusQueued || child.Status == task.StatusRunning
			running = running || child.Status == task.StatusRunning
			artwork = artwork || (child.Type == string(tasks.KindCover) && child.Status == task.StatusRunning)
			if child.Status == task.StatusFailed {
				failed = true
				if info.Error == nil {
					info.Error = child.Error
				}
			}
			if child.UpdatedAt.After(info.UpdatedAt) {
				info.UpdatedAt = child.UpdatedAt
			}
		}
		if info.Scan.MetadataTotal > 0 && info.Status != string(task.StatusFailed) {
			info.Progress = info.Scan.MetadataCompleted * 100 / info.Scan.MetadataTotal
			if active {
				info.Status, info.Scan.Stage = string(task.StatusQueued), "scraping"
				if running {
					info.Status = string(task.StatusRunning)
				}
				if artwork {
					info.Scan.Stage = "artwork"
				}
			} else if failed {
				info.Status = string(task.StatusFailed)
			}
		}
		info.CanRetry = info.Status == string(task.StatusFailed)
		result = append(result, info)
	}
	return result, nil
}

// Return one row per parent/type/status, with its count and latest change.
// Keep large cover documents inside SQLite instead of decoding every child.
func metadataGroups(ctx context.Context, database *ent.Client, records []*ent.Task) ([]metadataTaskGroup, error) {
	children := sql.Table(task.Table)
	parents := make([]any, 0, len(records))
	for _, record := range records {
		parents = append(parents, record.ID)
	}
	parent := tasks.JSONExtract(children.C(task.FieldPayload), "scan_task_id")
	partition := "PARTITION BY " + parent + ", " + children.C(task.FieldType) + ", " + children.C(task.FieldStatus)
	groups := sql.Select(
		children.C(task.FieldID), sql.As(parent, "parent_id"),
		sql.As("COUNT(*) OVER ("+partition+")", "count"),
		sql.As("ROW_NUMBER() OVER ("+partition+" ORDER BY "+children.C(task.FieldUpdatedAt)+" DESC, "+children.C(task.FieldID)+" DESC)", "position"),
	).From(children).Where(sql.And(sql.In(children.C(task.FieldType), string(tasks.KindScrape), string(tasks.KindCover)),
		sqljson.ValueIn(children.C(task.FieldPayload), parents, sqljson.Path("scan_task_id")))).As("metadata_groups")
	var result []metadataTaskGroup
	err := database.Task.Query().Where(func(s *sql.Selector) {
		s.Join(groups).On(s.C(task.FieldID), groups.C(task.FieldID))
		s.Where(sql.EQ(groups.C("position"), 1))
		s.Select(s.C(task.FieldType), s.C(task.FieldStatus), s.C(task.FieldError), s.C(task.FieldUpdatedAt), groups.C("parent_id"), groups.C("count"))
	}).Select(task.FieldID).Scan(ctx, &result)
	return result, err
}

// scanTaskInfo extracts domain.TaskInfo from a scan task ent.Task record.
func scanTaskInfo(record *ent.Task) (domain.TaskInfo, error) {
	payload, err := tasks.DecodePayload[domain.ScanPayload](record.Payload)
	if err != nil {
		return domain.TaskInfo{}, fmt.Errorf("read scan task %d: %w", record.ID, err)
	}
	return domain.TaskInfo{
		ID: record.ID, Type: record.Type, Status: string(record.Status), Progress: record.Progress,
		Error: record.Error, CanRetry: record.Status == task.StatusFailed, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		Source: payload.Source, Scan: payload.Scan, OfflineTaskID: payload.OfflineTaskID,
	}, nil
}

// ensureScanTask finds an existing reusable scan task for the source or creates a new queued scan.
func ensureScanTask(ctx context.Context, taskClient *ent.TaskClient, source domain.LibrarySource, reusable ...task.Status) (*ent.Task, error) {
	record, err := taskClient.Query().Where(
		task.TypeEQ(string(tasks.KindScan)), task.StatusIn(reusable...),
		func(selector *sql.Selector) {
			selector.Where(sql.And(
				sql.Not(sqljson.HasKey(task.FieldPayload, sqljson.Path("target_id"))),
				sqljson.ValueEQ(task.FieldPayload, source.AccountID, sqljson.Path("source", "account_id")),
				sqljson.ValueEQ(task.FieldPayload, source.Directory.ID, sqljson.Path("source", "directory", "id")),
			))
		},
	).First(ctx)
	if err == nil {
		return record, nil
	}
	if !ent.IsNotFound(err) {
		return nil, fmt.Errorf("find active scan: %w", err)
	}
	payload, err := tasks.EncodePayload(domain.ScanPayload{
		Source: source, Scan: domain.ScanProgress{Stage: "queued", CurrentPath: source.Directory.Path},
	})
	if err != nil {
		return nil, err
	}
	record, err = taskClient.Create().SetType(string(tasks.KindScan)).SetPayload(payload).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("queue library scan: %w", err)
	}
	return record, nil
}

// EnqueueScan reuses a queued or running full scan of the source or creates
// one. It serves manual retries, where any active scan is good enough.
func (s *Service) EnqueueScan(ctx context.Context, source domain.LibrarySource) (domain.TaskInfo, error) {
	return s.enqueueScan(ctx, source, task.StatusQueued, task.StatusRunning)
}

// EnqueueFreshScan queues a full scan that observes the current mount. Only a
// scan that has not started yet is reused: a running one may have been issued
// against an earlier mount of the same directory and is about to fail its
// source check.
func (s *Service) EnqueueFreshScan(ctx context.Context, source domain.LibrarySource) (domain.TaskInfo, error) {
	return s.enqueueScan(ctx, source, task.StatusQueued)
}

func (s *Service) enqueueScan(ctx context.Context, source domain.LibrarySource, reusable ...task.Status) (domain.TaskInfo, error) {
	if err := s.tasks.Queue().Lock(ctx); err != nil {
		return domain.TaskInfo{}, err
	}
	defer s.tasks.Queue().Unlock()
	record, err := ensureScanTask(ctx, s.database.Task, source, reusable...)
	if err != nil {
		return domain.TaskInfo{}, err
	}
	s.tasks.NotifyUI()
	if record.Status == task.StatusQueued {
		s.tasks.WakePool()
	}
	return scanTaskInfo(record)
}
