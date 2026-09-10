package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/setting"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/pan"
)

const panDirectorySetting = "pan.library_directory"

type PanLibraryDirectory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type panLibraryDirectory struct {
	AccountID string `json:"account_id"`
	PanLibraryDirectory
}

func (service *PanService) Files(ctx context.Context, directoryID string, page int) (pan.FilePage, error) {
	state := service.snapshot()
	files, err := withPanToken(ctx, service, state, func(token string) (pan.FilePage, error) {
		return service.client.List(ctx, token, directoryID, (page-1)*100, 100)
	})
	if err != nil {
		return pan.FilePage{}, fmt.Errorf("list 115 directory: %w", err)
	}
	if _, err := service.credentials(state); err != nil {
		return pan.FilePage{}, err
	}
	return files, nil
}

func (service *PanService) SelectDirectory(ctx context.Context, directoryID string) (PanLibraryDirectory, error) {
	state := service.snapshot()
	account, err := service.account(ctx, state)
	if err != nil {
		return PanLibraryDirectory{}, fmt.Errorf("get 115 account for directory: %w", err)
	}
	files, err := withPanToken(ctx, service, state, func(token string) (pan.FilePage, error) {
		return service.client.List(ctx, token, directoryID, 0, 1)
	})
	if err != nil {
		return PanLibraryDirectory{}, fmt.Errorf("get 115 media directory: %w", err)
	}
	names := make([]string, 0, len(files.Path))
	for _, directory := range files.Path {
		if directory.ID != "0" {
			names = append(names, directory.Name)
		}
	}
	directory := PanLibraryDirectory{
		ID: directoryID, Name: files.Path[len(files.Path)-1].Name,
		Path: "/" + strings.Join(names, "/"),
	}
	record := panLibraryDirectory{AccountID: account.ID, PanLibraryDirectory: directory}
	if err := service.commit.Lock(ctx); err != nil {
		return PanLibraryDirectory{}, err
	}
	defer service.commit.Unlock()
	current, err := service.credentials(state)
	if err != nil {
		return PanLibraryDirectory{}, err
	}
	if current.directory.AccountID == record.AccountID && current.directory.ID == record.ID {
		// Retried requests for the current mount must not invalidate active work.
		return current.directory.PanLibraryDirectory, nil
	}
	if _, err := service.sourceState(state.source(), state.authorizationVersion); err != nil {
		return PanLibraryDirectory{}, err
	}
	if err := service.tasks.queue.Lock(ctx); err != nil {
		return PanLibraryDirectory{}, err
	}
	defer service.tasks.queue.Unlock()
	if err := ent.WithTx(ctx, service.database, func(tx *ent.Tx) error {
		if err := saveSetting(ctx, tx.Client(), panDirectorySetting, record); err != nil {
			return err
		}
		_, err := ensureScanTask(ctx, tx.Task, LibrarySource{AccountID: account.ID, Directory: directory}, task.StatusQueued)
		return err
	}); err != nil {
		return PanLibraryDirectory{}, fmt.Errorf("mount media directory and queue scan: %w", err)
	}
	service.mu.Lock()
	service.directory = record
	service.authorizationVersion++
	service.mu.Unlock()
	service.tasks.NotifyLibraryChanged()
	return directory, nil
}

func (service *PanService) ClearDirectory(ctx context.Context) error {
	if err := service.commit.Lock(ctx); err != nil {
		return err
	}
	defer service.commit.Unlock()
	return service.clearDirectory(ctx)
}

// The caller holds commit after verifying the current account with 115.
func (service *PanService) discardOtherAccountDirectory(ctx context.Context, accountID string) error {
	directory := service.snapshot().directory
	if directory.ID == "" || directory.AccountID == accountID {
		return nil
	}
	return service.clearDirectory(ctx)
}

func (service *PanService) clearDirectory(ctx context.Context) error {
	if _, err := service.database.Setting.Delete().Where(setting.Key(panDirectorySetting)).Exec(ctx); err != nil {
		return fmt.Errorf("remove 115 media directory setting: %w", err)
	}
	service.mu.Lock()
	service.directory = panLibraryDirectory{}
	service.authorizationVersion++
	service.mu.Unlock()
	return nil
}
