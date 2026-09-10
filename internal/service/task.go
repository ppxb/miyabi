package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
)

type TaskInfo struct {
	ID            int           `json:"id"`
	Type          string        `json:"type"`
	Status        task.Status   `json:"status"`
	Progress      int           `json:"progress"`
	Error         *string       `json:"error,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	Source        LibrarySource `json:"source"`
	Scan          ScanProgress  `json:"scan"`
	OfflineTaskID int           `json:"offline_task_id,omitempty"`
}

// TaskJob is internal execution input. API responses never expose raw payloads.
type TaskJob struct {
	ID      int
	Type    string
	Payload map[string]any
}

type TaskRevisions struct {
	Library uint64 `json:"library"`
	Offline uint64 `json:"offline"`
}

type TaskService struct {
	revisions   TaskRevisions
	database    *ent.Client
	enqueueMu   sync.Mutex
	wake        chan struct{}
	mu          sync.Mutex
	subscribers map[chan struct{}]struct{}
}

func NewTaskService(database *ent.Client) *TaskService {
	return &TaskService{
		database: database, wake: make(chan struct{}, 1),
		subscribers: make(map[chan struct{}]struct{}),
	}
}

func (service *TaskService) enqueueScan(ctx context.Context, source LibrarySource) (TaskInfo, error) {
	service.enqueueMu.Lock()
	defer service.enqueueMu.Unlock()
	record, err := service.database.Task.Query().Where(
		task.TypeEQ("scan"), task.StatusIn(task.StatusQueued, task.StatusRunning),
		func(selector *sql.Selector) {
			selector.Where(sql.And(
				sql.Not(sqljson.HasKey(task.FieldPayload, sqljson.Path("target_id"))),
				sqljson.ValueEQ(task.FieldPayload, source.AccountID, sqljson.Path("source", "account_id")),
				sqljson.ValueEQ(task.FieldPayload, source.Directory.ID, sqljson.Path("source", "directory", "id")),
			))
		},
	).First(ctx)
	if err == nil {
		return scanTaskInfo(record)
	}
	if !ent.IsNotFound(err) {
		return TaskInfo{}, fmt.Errorf("find active scan: %w", err)
	}
	payload := (scanPayload{
		Source: source, Scan: ScanProgress{Stage: "queued", CurrentPath: source.Directory.Path},
	}).taskPayload()
	record, err = service.database.Task.Create().SetType("scan").SetPayload(payload).Save(ctx)
	if err != nil {
		return TaskInfo{}, fmt.Errorf("queue library scan: %w", err)
	}
	service.Notify()
	return scanTaskInfo(record)
}

// The UI shows one workflow per scan. Metadata and artwork jobs remain durable
// individual tasks while their progress is folded into that compact workflow.
func (service *TaskService) List(ctx context.Context) ([]TaskInfo, error) {
	records, err := service.database.Task.Query().Where(task.TypeEQ("scan")).
		Order(ent.Desc(task.FieldID)).Limit(20).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list scan tasks: %w", err)
	}
	active, err := service.database.Task.Query().Where(task.TypeIn("scan", "scrape", "cover"),
		task.StatusIn(task.StatusQueued, task.StatusRunning), func(s *sql.Selector) {
			s.Select("CASE WHEN type = 'scan' THEN id ELSE json_extract(payload, '$.scan_task_id') END").Distinct()
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
		parents, err := service.database.Task.Query().Where(task.IDIn(missing...)).All(ctx)
		if err != nil {
			return nil, err
		}
		records = append(records, parents...)
	}
	result, err := service.workflowInfos(ctx, records)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(result, func(a, b TaskInfo) int {
		aActive := a.Status == task.StatusQueued || a.Status == task.StatusRunning
		bActive := b.Status == task.StatusQueued || b.Status == task.StatusRunning
		if aActive != bActive {
			if aActive {
				return -1
			}
			return 1
		}
		if a.Status == task.StatusRunning && b.Status == task.StatusQueued {
			return -1
		}
		if a.Status == task.StatusQueued && b.Status == task.StatusRunning {
			return 1
		}
		return b.ID - a.ID
	})
	return result, nil
}

type metadataTaskGroup struct {
	ParentID  int         `json:"parent_id"`
	Count     int         `json:"count"`
	Type      string      `json:"type"`
	Status    task.Status `json:"status"`
	Error     *string     `json:"error"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// Return one row per parent/type/status, with its count and latest change.
// Keep large cover documents inside SQLite instead of decoding every child.
func (service *TaskService) metadataGroups(ctx context.Context, records []*ent.Task) ([]metadataTaskGroup, error) {
	children := sql.Table(task.Table)
	parents := make([]any, 0, len(records))
	for _, record := range records {
		parents = append(parents, record.ID)
	}
	parent := "json_extract(" + children.C(task.FieldPayload) + ", '$.scan_task_id')"
	partition := "PARTITION BY " + parent + ", " + children.C(task.FieldType) + ", " + children.C(task.FieldStatus)
	groups := sql.Select(
		children.C(task.FieldID), sql.As(parent, "parent_id"),
		sql.As("COUNT(*) OVER ("+partition+")", "count"),
		sql.As("ROW_NUMBER() OVER ("+partition+" ORDER BY "+children.C(task.FieldUpdatedAt)+" DESC, "+children.C(task.FieldID)+" DESC)", "position"),
	).From(children).Where(sql.And(sql.In(children.C(task.FieldType), "scrape", "cover"),
		sqljson.ValueIn(children.C(task.FieldPayload), parents, sqljson.Path("scan_task_id")))).As("metadata_groups")
	var result []metadataTaskGroup
	err := service.database.Task.Query().Where(func(s *sql.Selector) {
		s.Join(groups).On(s.C(task.FieldID), groups.C(task.FieldID))
		s.Where(sql.EQ(groups.C("position"), 1))
		s.Select(s.C(task.FieldType), s.C(task.FieldStatus), s.C(task.FieldError), s.C(task.FieldUpdatedAt), groups.C("parent_id"), groups.C("count"))
	}).Select(task.FieldID).Scan(ctx, &result)
	return result, err
}

func (service *TaskService) Info(ctx context.Context, id int) (TaskInfo, error) {
	record, err := service.database.Task.Get(ctx, id)
	if err != nil {
		return TaskInfo{}, err
	}
	infos, err := service.workflowInfos(ctx, []*ent.Task{record})
	if err != nil {
		return TaskInfo{}, err
	}
	return infos[0], nil
}

func (service *TaskService) workflowInfos(ctx context.Context, records []*ent.Task) ([]TaskInfo, error) {
	result := make([]TaskInfo, 0, len(records))
	if len(records) == 0 {
		return result, nil
	}
	children, err := service.metadataGroups(ctx, records)
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
			if child.Type == "scrape" {
				info.Scan.MetadataTotal += child.Count
			}
			if (child.Type == "cover" && child.Status == task.StatusDone) || child.Status == task.StatusFailed {
				info.Scan.MetadataCompleted += child.Count
			}
			active = active || child.Status == task.StatusQueued || child.Status == task.StatusRunning
			running = running || child.Status == task.StatusRunning
			artwork = artwork || (child.Type == "cover" && child.Status == task.StatusRunning)
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
		if info.Scan.MetadataTotal > 0 && info.Status != task.StatusFailed {
			info.Progress = info.Scan.MetadataCompleted * 100 / info.Scan.MetadataTotal
			if active {
				info.Status, info.Scan.Stage = task.StatusQueued, "scraping"
				if running {
					info.Status = task.StatusRunning
				}
				if artwork {
					info.Scan.Stage = "artwork"
				}
			} else if failed {
				info.Status = task.StatusFailed
			}
		}
		result = append(result, info)
	}
	return result, nil
}

func scanTaskInfo(record *ent.Task) (TaskInfo, error) {
	payload, err := decodeTaskPayload[scanPayload](record.Payload)
	if err != nil {
		return TaskInfo{}, fmt.Errorf("read scan task %d: %w", record.ID, err)
	}
	return TaskInfo{
		ID: record.ID, Type: record.Type, Status: record.Status, Progress: record.Progress,
		Error: record.Error, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		Source: payload.Source, Scan: payload.Scan, OfflineTaskID: payload.OfflineTaskID,
	}, nil
}

func (service *TaskService) Recover(ctx context.Context, types []string) error {
	if _, err := service.database.Task.Update().Where(
		task.TypeIn(types...), task.StatusEQ(task.StatusRunning),
	).SetStatus(task.StatusQueued).SetProgress(0).ClearError().Save(ctx); err != nil {
		return fmt.Errorf("recover interrupted tasks: %w", err)
	}
	service.Notify()
	return nil
}

func (service *TaskService) Claim(ctx context.Context, types []string) (*TaskJob, error) {
	for {
		record, err := service.database.Task.Query().Where(
			task.TypeIn(types...), task.StatusEQ(task.StatusQueued),
		).Order(ent.Asc(task.FieldID)).First(ctx)
		if ent.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("find queued task: %w", err)
		}
		claimed, err := service.database.Task.Update().Where(
			task.IDEQ(record.ID), task.StatusEQ(task.StatusQueued),
		).SetStatus(task.StatusRunning).SetProgress(0).ClearError().Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("claim task %d: %w", record.ID, err)
		}
		if claimed == 0 {
			continue
		}
		service.Notify()
		return &TaskJob{ID: record.ID, Type: record.Type, Payload: record.Payload}, nil
	}
}

func (service *TaskService) Finish(ctx context.Context, id int, runError error) error {
	libraryChanged := false
	if err := ent.WithTx(ctx, service.database, func(tx *ent.Tx) error {
		record, err := tx.Task.Get(ctx, id)
		if err != nil {
			return err
		}
		update := tx.Task.UpdateOneID(id)
		if runError != nil {
			update.SetStatus(task.StatusFailed).SetError(runError.Error())
			if record.Type == "scrape" || record.Type == "cover" {
				libraryChanged = true
				input, err := decodeTaskPayload[metadataPayload](record.Payload)
				if err != nil {
					return err
				}
				if err := tx.Movie.Update().Where(movie.IDEQ(input.MovieID), movie.ScrapeStatusNEQ(movie.ScrapeStatusDone),
					movie.HasFilesWith(libraryFiles(input.Source))).SetScrapeStatus(movie.ScrapeStatusFailed).Exec(ctx); err != nil {
					return err
				}
			}
		} else {
			update.SetStatus(task.StatusDone).SetProgress(100).ClearError()
		}
		return update.Exec(ctx)
	}); err != nil {
		return fmt.Errorf("finish task %d: %w", id, err)
	}
	if libraryChanged {
		service.NotifyLibraryChanged()
	} else {
		service.NotifyOfflineChanged()
	}
	return nil
}

func (service *TaskService) Pending() <-chan struct{} {
	return service.wake
}

// Notifications are coalesced. Each consumer reads a fresh database snapshot,
// so a slow SSE client cannot block workers or accumulate progress events.
func (service *TaskService) Subscribe() (<-chan struct{}, func()) {
	updates := make(chan struct{}, 1)
	service.mu.Lock()
	service.subscribers[updates] = struct{}{}
	service.mu.Unlock()
	return updates, func() {
		service.mu.Lock()
		delete(service.subscribers, updates)
		service.mu.Unlock()
	}
}

func (service *TaskService) Notify() {
	service.notify(false, false)
}

func (service *TaskService) NotifyLibraryChanged() {
	service.notify(true, false)
}

func (service *TaskService) NotifyOfflineChanged() {
	service.notify(false, true)
}

func (service *TaskService) Revisions() TaskRevisions {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.revisions
}

func (service *TaskService) notify(library, offline bool) {
	select {
	case service.wake <- struct{}{}:
	default:
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if library {
		service.revisions.Library++
	}
	if offline {
		service.revisions.Offline++
	}
	for subscriber := range service.subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

func encodeTaskPayload(value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode task payload: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return nil, fmt.Errorf("decode task payload object: %w", err)
	}
	return payload, nil
}

func decodeTaskPayload[T any](payload map[string]any) (T, error) {
	var value T
	encoded, err := json.Marshal(payload)
	if err != nil {
		return value, fmt.Errorf("encode stored task: %w", err)
	}
	if err := json.Unmarshal(encoded, &value); err != nil {
		return value, fmt.Errorf("decode stored task: %w", err)
	}
	return value, nil
}
