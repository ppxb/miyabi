package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/ent"
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
	Progress int         `json:"progress"`
	Error    *string     `json:"error,omitempty"`
}

type offlinePayload struct {
	Code        string `json:"code"`
	JavDBID     string `json:"javdb_id"`
	Hash        string `json:"hash"`
	InfoHash    string `json:"info_hash"`
	AccountID   string `json:"account_id"`
	DirectoryID string `json:"directory_id"`
}

type OfflineService struct {
	database *ent.Client
	discover *DiscoverService
	drive    *PanService
}

func NewOfflineService(database *ent.Client, discover *DiscoverService, drive *PanService) *OfflineService {
	return &OfflineService{database: database, discover: discover, drive: drive}
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

	// Keep the account, destination, and submission together. This also prevents
	// simultaneous clicks from submitting the same active download twice.
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
	existing, err := service.database.Task.Query().Where(
		task.TypeEQ("offline"),
		task.StatusIn(task.StatusQueued, task.StatusRunning),
		func(selector *sql.Selector) {
			selector.Where(sql.And(
				sqljson.ValueEQ(task.FieldPayload, account.ID, sqljson.Path("account_id")),
				sqljson.ValueEQ(task.FieldPayload, hash, sqljson.Path("hash")),
			))
		},
	).First(ctx)
	if err == nil {
		return offlineSubmission(existing, hash), nil
	}
	if !ent.IsNotFound(err) {
		return OfflineSubmission{}, fmt.Errorf("find active offline task: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return OfflineSubmission{}, err
	}

	// A successful remote submission must still be recorded if the caller leaves.
	submitContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
	defer cancel()
	infoHash, err := withPanToken(submitContext, service.drive, func(token string) (string, error) {
		return service.drive.client.AddOffline(submitContext, token, "magnet:?xt=urn:btih:"+hash, directory.ID)
	})
	if err != nil {
		return OfflineSubmission{}, fmt.Errorf("submit 115 offline download: %w", err)
	}
	created, err := service.database.Task.Create().
		SetType("offline").SetStatus(task.StatusRunning).
		SetPayload(map[string]any{
			"code": code, "javdb_id": movie.ID, "hash": hash, "info_hash": infoHash,
			"account_id": account.ID, "directory_id": directory.ID,
		}).Save(submitContext)
	if err != nil {
		return OfflineSubmission{}, fmt.Errorf("record 115 offline download: %w", err)
	}
	return offlineSubmission(created, hash), nil
}

func offlineSubmission(record *ent.Task, hash string) OfflineSubmission {
	return OfflineSubmission{
		TaskID: record.ID, Hash: hash, Status: record.Status,
		Progress: record.Progress, Error: record.Error,
	}
}

// Tasks returns the latest attempt for each magnet from local records only.
func (service *OfflineService) Tasks(ctx context.Context, movieID, accountID string) ([]OfflineSubmission, error) {
	records, err := service.database.Task.Query().Where(
		task.TypeEQ("offline"),
		func(selector *sql.Selector) {
			selector.Where(sql.And(
				sqljson.ValueEQ(task.FieldPayload, movieID, sqljson.Path("javdb_id")),
				sqljson.ValueEQ(task.FieldPayload, accountID, sqljson.Path("account_id")),
			))
		},
	).Order(ent.Desc(task.FieldID)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load movie offline tasks: %w", err)
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
		result = append(result, offlineSubmission(record, hash))
	}
	return result, nil
}

// Sync updates only downloads accepted by 115. Completed downloads do not
// create Movie records; those belong to the subsequent library scan.
func (service *OfflineService) Sync(ctx context.Context) error {
	records, err := service.database.Task.Query().Where(
		task.TypeEQ("offline"), task.StatusIn(task.StatusQueued, task.StatusRunning),
	).All(ctx)
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
	account, err := withPanToken(ctx, service.drive, func(token string) (pan.Account, error) {
		return service.drive.client.Account(ctx, token)
	})
	if err != nil {
		return fmt.Errorf("get 115 account for offline sync: %w", err)
	}
	wanted := make(map[string]*ent.Task)
	for _, record := range records {
		encoded, err := json.Marshal(record.Payload)
		if err != nil {
			return fmt.Errorf("encode offline task %d: %w", record.ID, err)
		}
		var payload offlinePayload
		if err := json.Unmarshal(encoded, &payload); err != nil {
			return fmt.Errorf("decode offline task %d: %w", record.ID, err)
		}
		if payload.AccountID == "" || payload.InfoHash == "" {
			return fmt.Errorf("offline task %d is missing account or remote task ID", record.ID)
		}
		if payload.AccountID == account.ID {
			wanted[strings.ToLower(payload.InfoHash)] = record
		}
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
		if err := service.database.Task.UpdateOneID(record.ID).
			SetStatus(task.StatusFailed).
			SetError("115 中未找到该任务，请在 115 客户端确认下载结果").Exec(ctx); err != nil {
			return fmt.Errorf("mark missing offline task %d: %w", record.ID, err)
		}
	}
	return nil
}

func (service *OfflineService) updateTask(ctx context.Context, record *ent.Task, remote pan.OfflineTask) error {
	status, progress := task.StatusRunning, remote.Progress
	switch remote.Status {
	case 0, 1:
	case 2:
		status, progress = task.StatusDone, 100
	case -1:
		status = task.StatusFailed
	default:
		return fmt.Errorf("115 returned unknown offline status %d", remote.Status)
	}
	if status == record.Status && progress == record.Progress {
		return nil
	}
	update := service.database.Task.UpdateOneID(record.ID).SetStatus(status).SetProgress(progress)
	if status == task.StatusDone {
		record.Payload["file_id"] = remote.FileID
		update.SetPayload(record.Payload)
	}
	if status == task.StatusFailed {
		update.SetError("115 离线下载失败，请在 115 客户端查看原因")
	}
	if err := update.Exec(ctx); err != nil {
		return fmt.Errorf("update offline task %d: %w", record.ID, err)
	}
	return nil
}
