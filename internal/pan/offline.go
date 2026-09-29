package pan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type OfflineTask struct {
	Hash        string
	Status      int
	Progress    int
	FileID      string
	DirectoryID string
}

type offlineTaskWire struct {
	Hash        string      `json:"info_hash"`
	Status      int         `json:"status"`
	Progress    json.Number `json:"percentDone"`
	FileID      string      `json:"file_id"`
	DirectoryID string      `json:"wp_path_id"`
}

// 115 can return fractional percentages or numeric strings alongside integers.
func (wire offlineTaskWire) task() (OfflineTask, error) {
	var progress float64
	if wire.Progress != "" {
		var err error
		progress, err = wire.Progress.Float64()
		if err != nil {
			return OfflineTask{}, fmt.Errorf("decode 115 offline progress: %w", err)
		}
	}
	return OfflineTask{
		Hash: wire.Hash, Status: wire.Status, Progress: int(max(0, min(100, progress))),
		FileID: wire.FileID, DirectoryID: wire.DirectoryID,
	}, nil
}

// RemoveOffline removes download history only. Source files are never deleted.
func (client *Client) RemoveOffline(ctx context.Context, accessToken, hash string) error {
	_, err := apiRequest[apiResponse](
		client,
		client.http.R().SetContext(ctx).SetAuthToken(accessToken).SetFormData(map[string]string{
			"info_hash": hash, "del_source_file": "0",
		}),
		http.MethodPost,
		apiURL+"/open/offline/del_task",
		"offline removal",
	)
	return err
}

type OfflinePage struct {
	PageCount int
	Tasks     []OfflineTask
}

func (client *Client) AddOffline(ctx context.Context, accessToken, uri, directoryID string) (string, error) {
	type addOfflineWire struct {
		apiResponse
		Data []struct {
			apiResponse
			Hash string `json:"info_hash"`
		} `json:"data"`
	}
	result, err := apiRequest[addOfflineWire](
		client,
		client.http.R().SetContext(ctx).SetAuthToken(accessToken).SetMultipartFormData(map[string]string{
			"urls": uri, "wp_path_id": directoryID,
		}),
		http.MethodPost,
		apiURL+"/open/offline/add_task_urls",
		"offline submission",
	)
	if err != nil {
		return "", err
	}
	if len(result.Data) != 1 {
		return "", fmt.Errorf("115 returned %d results for one offline task", len(result.Data))
	}
	item := result.Data[0]
	if err := item.err(); err != nil {
		return "", err
	}
	if item.Hash == "" {
		return "", fmt.Errorf("115 offline submission is missing info_hash")
	}
	return item.Hash, nil
}

func (client *Client) OfflineTasks(ctx context.Context, accessToken string, page int) (OfflinePage, error) {
	type offlineTasksWire struct {
		apiResponse
		Data *struct {
			PageCount int               `json:"page_count"`
			Tasks     []offlineTaskWire `json:"tasks"`
		} `json:"data"`
	}
	result, err := apiRequest[offlineTasksWire](
		client,
		client.http.R().SetContext(ctx).SetAuthToken(accessToken).SetQueryParam("page", strconv.Itoa(page)),
		http.MethodGet,
		apiURL+"/open/offline/get_task_list",
		"offline tasks",
	)
	if err != nil {
		return OfflinePage{}, err
	}
	if result.Data == nil {
		return OfflinePage{}, fmt.Errorf("115 offline task list is missing data")
	}
	pageResult := OfflinePage{PageCount: result.Data.PageCount}
	if result.Data.Tasks != nil {
		pageResult.Tasks = make([]OfflineTask, len(result.Data.Tasks))
	}
	for i, wire := range result.Data.Tasks {
		task, err := wire.task()
		if err != nil {
			return OfflinePage{}, err
		}
		pageResult.Tasks[i] = task
	}
	return pageResult, nil
}
