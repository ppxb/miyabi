package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/pan"
)

var (
	ErrMediaDirectoryRequired = errors.New("请先在设置页挂载当前 115 账号的媒体目录")
	ErrMagnetNotFound         = errors.New("磁力链不属于当前影片，请刷新后重试")
)

type OfflineSubmission struct {
	TaskID   int         `json:"task_id"`
	Hash     string      `json:"hash"`
	Status   task.Status `json:"status"`
	Phase    string      `json:"phase"`
	Progress int         `json:"progress"`
	Error    *string     `json:"error,omitempty"`
}

type offlinePayload struct {
	Code        string   `json:"code"`
	JavDBID     string   `json:"javdb_id"`
	Hash        string   `json:"hash"`
	InfoHash    string   `json:"info_hash"`
	AccountID   string   `json:"account_id"`
	DirectoryID string   `json:"directory_id"`
	FileID      string   `json:"file_id,omitempty"`
	FileIDs     []string `json:"file_ids,omitempty"`
	ScanTaskID  int      `json:"scan_task_id,omitempty"`
}

type OfflineService struct {
	database *ent.Client
	discover *DiscoverService
	drive    *PanService
	tasks    *TaskService
}

func NewOfflineService(database *ent.Client, discover *DiscoverService, drive *PanService, tasks *TaskService) *OfflineService {
	return &OfflineService{database: database, discover: discover, drive: drive, tasks: tasks}
}

func (service *OfflineService) Add(ctx context.Context, movieID, hash string) (OfflineSubmission, error) {
	hash = strings.ToLower(hash)
	magnets, err := service.discover.Magnets(ctx, movieID)
	if err != nil {
		return OfflineSubmission{}, err
	}
	if !slices.ContainsFunc(magnets, func(magnet DiscoverMagnet) bool { return magnet.Hash == hash }) {
		return OfflineSubmission{}, ErrMagnetNotFound
	}
	movie, err := service.discover.MovieDetail(ctx, movieID)
	if err != nil {
		return OfflineSubmission{}, err
	}
	code := codeid.Normalize(movie.Code)
	if code == "" {
		return OfflineSubmission{}, fmt.Errorf("normalize offline movie code %q", movie.Code)
	}
	service.drive.mu.Lock()
	defer service.drive.mu.Unlock()
	account, err := withPanToken(ctx, service.drive, func(token string) (pan.Account, error) {
		return service.drive.client.Account(ctx, token)
	})
	if err != nil {
		return OfflineSubmission{}, fmt.Errorf("get 115 account for offline download: %w", err)
	}
	directory := service.drive.directory
	if directory.ID == "" || directory.AccountID != account.ID {
		return OfflineSubmission{}, ErrMediaDirectoryRequired
	}
	source := LibrarySource{AccountID: account.ID, Directory: directory.PanLibraryDirectory}
	existing, err := service.database.Task.Query().Where(task.TypeEQ("offline"),
		task.StatusIn(task.StatusQueued, task.StatusRunning), func(s *sql.Selector) {
			s.Where(sql.And(
				sqljson.ValueEQ(task.FieldPayload, account.ID, sqljson.Path("account_id")),
				sqljson.ValueEQ(task.FieldPayload, hash, sqljson.Path("hash")),
			))
		}).First(ctx)
	if err == nil {
		if existing.Payload["directory_id"] != directory.ID {
			return OfflineSubmission{}, fmt.Errorf("该磁力正在下载到另一个目录，请先在 115 中处理该任务")
		}
		return service.submission(ctx, existing, &source)
	}
	if !ent.IsNotFound(err) {
		return OfflineSubmission{}, fmt.Errorf("find active offline task: %w", err)
	}
	previous, err := service.database.Task.Query().Where(task.TypeEQ("offline"), task.StatusEQ(task.StatusDone), func(s *sql.Selector) {
		s.Where(sql.And(
			sqljson.ValueEQ(task.FieldPayload, account.ID, sqljson.Path("account_id")),
			sqljson.ValueEQ(task.FieldPayload, directory.ID, sqljson.Path("directory_id")),
			sqljson.ValueEQ(task.FieldPayload, hash, sqljson.Path("hash")),
		))
	}).Order(ent.Desc(task.FieldID)).First(ctx)
	if err == nil {
		state, err := service.submission(ctx, previous, &source)
		if err != nil {
			return OfflineSubmission{}, err
		}
		if state.Phase == "processing" {
			return state, nil
		}
	} else if !ent.IsNotFound(err) {
		return OfflineSubmission{}, fmt.Errorf("find download workflow: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return OfflineSubmission{}, err
	}
	// Once a remote mutation starts, finish recording it even if the tab closes.
	submitContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	remote, err := service.submit(submitContext, source, hash)
	if err != nil {
		return OfflineSubmission{}, fmt.Errorf("submit 115 offline download: %w", err)
	}
	input := offlinePayload{Code: code, JavDBID: movie.ID, Hash: hash, InfoHash: remote.Hash,
		AccountID: account.ID, DirectoryID: directory.ID}
	encoded, err := encodeTaskPayload(input)
	if err != nil {
		return OfflineSubmission{}, err
	}
	var created *ent.Task
	if err := ent.WithTx(submitContext, service.database, func(tx *ent.Tx) error {
		var err error
		created, err = tx.Task.Create().SetType("offline").SetStatus(task.StatusRunning).SetPayload(encoded).Save(submitContext)
		if err != nil {
			return err
		}
		if remote.Status == 2 {
			return service.completeTask(submitContext, tx, created, input, remote.FileID)
		}
		return nil
	}); err != nil {
		return OfflineSubmission{}, fmt.Errorf("record 115 offline download: %w", err)
	}
	service.tasks.Notify()
	created, err = service.database.Task.Get(submitContext, created.ID)
	if err != nil {
		return OfflineSubmission{}, err
	}
	return service.submission(submitContext, created, &source)
}

// submit handles duplicate history by inspecting its real output. Only a
// terminal task with confirmed absent video content is removed, never files.
// The caller holds drive.mu throughout account validation and mutations.
func (service *OfflineService) submit(ctx context.Context, source LibrarySource, hash string) (pan.OfflineTask, error) {
	add := func() (string, error) {
		return withPanToken(ctx, service.drive, func(token string) (string, error) {
			return service.drive.client.AddOffline(ctx, token, "magnet:?xt=urn:btih:"+hash, source.Directory.ID)
		})
	}
	infoHash, err := add()
	if err == nil {
		return pan.OfflineTask{Hash: infoHash}, nil
	}
	if !errors.Is(err, pan.ErrOfflineExists) {
		return pan.OfflineTask{}, err
	}
	remote, err := service.findRemoteTask(ctx, hash)
	if err != nil {
		return pan.OfflineTask{}, err
	}
	if remote.Status == 0 || remote.Status == 1 {
		if remote.DirectoryID != source.Directory.ID {
			return pan.OfflineTask{}, fmt.Errorf("115 已有该磁力的下载任务，目标目录与当前媒体目录不一致")
		}
		return remote, nil
	}
	if remote.Status != 2 && remote.Status != -1 {
		return pan.OfflineTask{}, fmt.Errorf("115 returned unknown offline status %d", remote.Status)
	}
	if remote.FileID == "" {
		return pan.OfflineTask{}, fmt.Errorf("115 的历史任务未提供资源位置，请先在 115 客户端清理该任务记录")
	}
	present, err := service.remoteHasVideo(ctx, source, remote.FileID)
	if err != nil {
		return pan.OfflineTask{}, err
	}
	if present {
		if remote.Status == -1 {
			return pan.OfflineTask{}, fmt.Errorf("115 任务失败但目录内仍有视频，请先在 115 客户端确认完整性")
		}
		return remote, nil
	}
	if _, err := withPanToken(ctx, service.drive, func(token string) (struct{}, error) {
		return struct{}{}, service.drive.client.RemoveOffline(ctx, token, remote.Hash)
	}); err != nil {
		return pan.OfflineTask{}, fmt.Errorf("remove stale 115 task history: %w", err)
	}
	infoHash, err = add()
	return pan.OfflineTask{Hash: infoHash}, err
}

func (service *OfflineService) findRemoteTask(ctx context.Context, hash string) (pan.OfflineTask, error) {
	for page := 1; ; page++ {
		remote, err := withPanToken(ctx, service.drive, func(token string) (pan.OfflinePage, error) {
			return service.drive.client.OfflineTasks(ctx, token, page)
		})
		if err != nil {
			return pan.OfflineTask{}, fmt.Errorf("find duplicate 115 task: %w", err)
		}
		for _, download := range remote.Tasks {
			if strings.EqualFold(download.Hash, hash) {
				return download, nil
			}
		}
		if page >= remote.PageCount {
			break
		}
	}
	return pan.OfflineTask{}, fmt.Errorf("115 提示任务已存在，但任务列表中未找到它，请稍后重试")
}

func (service *OfflineService) remoteHasVideo(ctx context.Context, source LibrarySource, id string) (bool, error) {
	info, err := withPanToken(ctx, service.drive, func(token string) (pan.FileInfo, error) {
		return service.drive.client.Info(ctx, token, id)
	})
	if errors.Is(err, pan.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check existing 115 resource: %w", err)
	}
	if !withinSource(info, source) {
		return false, fmt.Errorf("该磁力的资源已在媒体目录之外，请先在 115 中移动资源")
	}
	if !info.IsDirectory {
		return isVideo(info.Name), nil
	}
	directories := []string{info.ID}
	seen := map[string]bool{info.ID: true}
	for next := 0; next < len(directories); next++ {
		total := -1
		for offset := 0; ; {
			page, err := withPanToken(ctx, service.drive, func(token string) (pan.FilePage, error) {
				return service.drive.client.List(ctx, token, directories[next], offset, 100)
			})
			if err != nil {
				return false, fmt.Errorf("check downloaded video files: %w", err)
			}
			if !slices.ContainsFunc(page.Path, func(dir pan.Directory) bool { return dir.ID == source.Directory.ID }) {
				return false, fmt.Errorf("下载目录已移出媒体目录")
			}
			if total == -1 {
				total = page.Total
			}
			if total != page.Total || (page.HasMore && len(page.Files) == 0) {
				return false, fmt.Errorf("下载目录读取不完整，请稍后重试")
			}
			for _, entry := range page.Files {
				if entry.IsDirectory {
					if !seen[entry.ID] {
						seen[entry.ID] = true
						directories = append(directories, entry.ID)
					}
				} else if isVideo(entry.Name) {
					return true, nil
				}
			}
			offset += len(page.Files)
			if !page.HasMore {
				if offset != total {
					return false, fmt.Errorf("下载目录分页不完整")
				}
				break
			}
		}
	}
	return false, nil
}

func (service *OfflineService) submission(ctx context.Context, record *ent.Task, source *LibrarySource) (OfflineSubmission, error) {
	input, err := decodeTaskPayload[offlinePayload](record.Payload)
	if err != nil {
		return OfflineSubmission{}, err
	}
	result := OfflineSubmission{TaskID: record.ID, Hash: input.Hash, Status: record.Status,
		Progress: record.Progress, Error: record.Error, Phase: "available"}
	if source == nil || source.AccountID != input.AccountID || source.Directory.ID != input.DirectoryID {
		return result, nil
	}
	if record.Status == task.StatusQueued || record.Status == task.StatusRunning {
		result.Phase = "downloading"
		return result, nil
	}
	if input.ScanTaskID != 0 {
		scan, err := service.tasks.Info(ctx, input.ScanTaskID)
		if err != nil {
			return OfflineSubmission{}, err
		}
		if scan.Status == task.StatusQueued || scan.Status == task.StatusRunning {
			result.Phase = "processing"
			return result, nil
		}
		if scan.Error != nil {
			result.Error = scan.Error
		}
	} else if record.Status == task.StatusDone && input.FileID != "" {
		result.Phase = "processing"
		return result, nil
	}
	for start := 0; start < len(input.FileIDs); start += 500 {
		present, err := service.database.File.Query().Where(libraryFiles(*source),
			file.FileIDIn(input.FileIDs[start:min(start+500, len(input.FileIDs))]...)).Exist(ctx)
		if err != nil {
			return OfflineSubmission{}, fmt.Errorf("read downloaded file index: %w", err)
		}
		if present {
			matched, err := service.database.File.Query().Where(libraryFiles(*source),
				file.FileIDIn(input.FileIDs[start:min(start+500, len(input.FileIDs))]...)).
				QueryMovie().Where(movie.CodeEQ(input.Code)).Select(movie.FieldScrapeStatus).Only(ctx)
			if err != nil && !ent.IsNotFound(err) {
				return OfflineSubmission{}, err
			}
			if err == nil {
				result.Phase = "in_library"
				if matched.ScrapeStatus != movie.ScrapeStatusFailed {
					result.Error = nil
				}
				break
			}
			result.Phase = "downloaded"
		}
	}
	return result, nil
}

// Tasks projects history through the current file index and workflow. A
// finished remote task alone never means the resource still exists.
func (service *OfflineService) Tasks(ctx context.Context, movieID, accountID string) ([]OfflineSubmission, error) {
	records, err := service.database.Task.Query().Where(task.TypeEQ("offline"), func(s *sql.Selector) {
		s.Where(sql.And(
			sqljson.ValueEQ(task.FieldPayload, movieID, sqljson.Path("javdb_id")),
			sqljson.ValueEQ(task.FieldPayload, accountID, sqljson.Path("account_id")),
		))
	}).Order(ent.Desc(task.FieldID)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load movie offline tasks: %w", err)
	}
	source, err := loadLibrarySource(ctx, service.database)
	if err != nil {
		return nil, err
	}
	result := make([]OfflineSubmission, 0, len(records))
	seen := make(map[string]bool)
	for _, record := range records {
		hash, ok := record.Payload["hash"].(string)
		if !ok || hash == "" {
			return nil, fmt.Errorf("offline task %d is missing magnet hash", record.ID)
		}
		if seen[hash] {
			continue
		}
		seen[hash] = true
		submission, err := service.submission(ctx, record, source)
		if err != nil {
			return nil, err
		}
		result = append(result, submission)
	}
	return result, nil
}

func (service *OfflineService) Sync(ctx context.Context) error {
	records, err := service.database.Task.Query().Where(task.TypeEQ("offline"), task.Or(
		task.StatusIn(task.StatusQueued, task.StatusRunning),
		task.And(task.StatusEQ(task.StatusDone), func(s *sql.Selector) {
			s.Where(sql.Not(sqljson.HasKey(task.FieldPayload, sqljson.Path("scan_task_id"))))
		}),
	)).All(ctx)
	if err != nil {
		return fmt.Errorf("load offline tasks: %w", err)
	}
	if len(records) == 0 {
		return nil
	}
	service.drive.mu.Lock()
	defer service.drive.mu.Unlock()
	if service.drive.tokens.AccessToken == "" {
		return nil
	}
	// Completed jobs awaiting another mount need no remote polling.
	records = slices.DeleteFunc(records, func(record *ent.Task) bool {
		return record.Status == task.StatusDone && (record.Payload["directory_id"] != service.drive.directory.ID ||
			record.Payload["account_id"] != service.drive.directory.AccountID)
	})
	if len(records) == 0 {
		return nil
	}
	account, err := withPanToken(ctx, service.drive, func(token string) (pan.Account, error) {
		return service.drive.client.Account(ctx, token)
	})
	if err != nil {
		return fmt.Errorf("get 115 account for offline sync: %w", err)
	}
	wanted := make(map[string]*ent.Task)
	for _, record := range records {
		input, err := decodeTaskPayload[offlinePayload](record.Payload)
		if err != nil {
			return err
		}
		if input.AccountID != account.ID {
			continue
		}
		if record.Status == task.StatusDone {
			if input.DirectoryID != service.drive.directory.ID || service.drive.directory.AccountID != account.ID {
				continue
			}
			if input.FileID != "" {
				if err := service.updateTask(ctx, record, pan.OfflineTask{Status: 2, FileID: input.FileID, Hash: input.InfoHash}); err != nil {
					return err
				}
				continue
			}
		}
		wanted[strings.ToLower(input.InfoHash)] = record
	}
	for page := 1; len(wanted) > 0; page++ {
		remote, err := withPanToken(ctx, service.drive, func(token string) (pan.OfflinePage, error) {
			return service.drive.client.OfflineTasks(ctx, token, page)
		})
		if err != nil {
			return fmt.Errorf("list 115 offline tasks: %w", err)
		}
		for _, download := range remote.Tasks {
			key := strings.ToLower(download.Hash)
			record, ok := wanted[key]
			if !ok {
				continue
			}
			if err := service.updateTask(ctx, record, download); err != nil {
				return err
			}
			delete(wanted, key)
		}
		if page >= remote.PageCount {
			break
		}
	}
	for _, record := range wanted {
		if err := service.database.Task.UpdateOneID(record.ID).SetStatus(task.StatusFailed).
			SetError("115 中未找到该任务，请在 115 客户端确认下载结果").Exec(ctx); err != nil {
			return err
		}
	}
	service.tasks.Notify()
	return nil
}

func (service *OfflineService) updateTask(ctx context.Context, record *ent.Task, remote pan.OfflineTask) error {
	status, progress := task.StatusRunning, remote.Progress
	switch remote.Status {
	case 0, 1:
	case 2:
		input, err := decodeTaskPayload[offlinePayload](record.Payload)
		if err != nil {
			return err
		}
		if err := ent.WithTx(ctx, service.database, func(tx *ent.Tx) error {
			return service.completeTask(ctx, tx, record, input, remote.FileID)
		}); err != nil {
			return fmt.Errorf("queue completed download scan: %w", err)
		}
		service.tasks.Notify()
		return nil
	case -1:
		status = task.StatusFailed
	default:
		return fmt.Errorf("115 returned unknown offline status %d", remote.Status)
	}
	if record.Status == status && record.Progress == progress {
		return nil
	}
	update := service.database.Task.UpdateOneID(record.ID).SetStatus(status).SetProgress(progress)
	if status == task.StatusFailed {
		update.SetError("115 离线下载失败，请在 115 客户端查看原因")
	}
	if err := update.Exec(ctx); err != nil {
		return fmt.Errorf("update offline task %d: %w", record.ID, err)
	}
	service.tasks.Notify()
	return nil
}

// Completion and targeted scan creation are one transaction. Never merge into
// a running scan: it might already have passed the newly downloaded directory.
func (service *OfflineService) completeTask(ctx context.Context, tx *ent.Tx, record *ent.Task, input offlinePayload, fileID string) error {
	if fileID == "" {
		return fmt.Errorf("115 下载已完成，但尚未返回文件位置")
	}
	input.FileID = fileID
	directory := service.drive.directory
	if input.ScanTaskID == 0 && directory.ID == input.DirectoryID && directory.AccountID == input.AccountID {
		encoded, err := encodeTaskPayload(scanPayload{
			Source:   LibrarySource{AccountID: input.AccountID, Directory: directory.PanLibraryDirectory},
			Scan:     ScanProgress{Stage: "queued", CurrentPath: directory.Path},
			TargetID: fileID, OfflineTaskID: record.ID, Code: input.Code, JavDBID: input.JavDBID,
		})
		if err != nil {
			return err
		}
		scan, err := tx.Task.Create().SetType("scan").SetPayload(encoded).Save(ctx)
		if err != nil {
			return err
		}
		input.ScanTaskID = scan.ID
	}
	encoded, err := encodeTaskPayload(input)
	if err != nil {
		return err
	}
	return tx.Task.UpdateOneID(record.ID).SetStatus(task.StatusDone).SetProgress(100).ClearError().SetPayload(encoded).Exec(ctx)
}
