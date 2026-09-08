package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/ppxb/miyabi/internal/ent/setting"
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
	service.mu.Lock()
	defer service.mu.Unlock()
	files, err := withPanToken(ctx, service, func(token string) (pan.FilePage, error) {
		return service.client.List(ctx, token, directoryID, (page-1)*100, 100)
	})
	if err != nil {
		return pan.FilePage{}, fmt.Errorf("list 115 directory: %w", err)
	}
	return files, nil
}

func (service *PanService) SelectDirectory(ctx context.Context, directoryID string) (PanLibraryDirectory, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	account, err := withPanToken(ctx, service, func(token string) (pan.Account, error) {
		return service.client.Account(ctx, token)
	})
	if err != nil {
		return PanLibraryDirectory{}, fmt.Errorf("get 115 account for directory: %w", err)
	}
	files, err := withPanToken(ctx, service, func(token string) (pan.FilePage, error) {
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
	if err := saveSetting(ctx, service.database, panDirectorySetting, record); err != nil {
		return PanLibraryDirectory{}, err
	}
	service.directory = record
	return directory, nil
}

func (service *PanService) ClearDirectory(ctx context.Context) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if _, err := service.database.Setting.Delete().Where(setting.Key(panDirectorySetting)).Exec(ctx); err != nil {
		return fmt.Errorf("remove 115 media directory setting: %w", err)
	}
	service.directory = panLibraryDirectory{}
	return nil
}
