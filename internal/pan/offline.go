package pan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type OfflineTask struct {
	Hash     string `json:"info_hash"`
	Status   int    `json:"status"`
	Progress int    `json:"percentDone"`
	FileID   string `json:"file_id"`
}

type OfflinePage struct {
	PageCount int           `json:"page_count"`
	Tasks     []OfflineTask `json:"tasks"`
}

func (client *Client) AddOffline(ctx context.Context, accessToken, uri, directoryID string) (string, error) {
	response, err := client.request(
		client.http.R().SetContext(ctx).SetAuthToken(accessToken).SetMultipartFormData(map[string]string{
			"urls": uri, "wp_path_id": directoryID,
		}),
		http.MethodPost, apiURL+"/open/offline/add_task_urls",
	)
	if err != nil {
		return "", err
	}
	var result struct {
		apiResponse
		Data []struct {
			apiResponse
			Hash string `json:"info_hash"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return "", fmt.Errorf("decode 115 offline submission: %w", err)
	}
	if err := result.err(); err != nil {
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
	response, err := client.request(
		client.http.R().SetContext(ctx).SetAuthToken(accessToken).SetQueryParam("page", strconv.Itoa(page)),
		http.MethodGet, apiURL+"/open/offline/get_task_list",
	)
	if err != nil {
		return OfflinePage{}, err
	}
	var result struct {
		apiResponse
		Data *OfflinePage `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return OfflinePage{}, fmt.Errorf("decode 115 offline tasks: %w", err)
	}
	if err := result.err(); err != nil {
		return OfflinePage{}, err
	}
	if result.Data == nil {
		return OfflinePage{}, fmt.Errorf("115 offline task list is missing data")
	}
	return *result.Data, nil
}
