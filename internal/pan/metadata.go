package pan

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type FileInfo struct {
	File
	Path []Directory
}

func (client *Client) Info(ctx context.Context, accessToken, fileID string) (FileInfo, error) {
	response, err := client.request(client.http.R().SetContext(ctx).SetAuthToken(accessToken).
		SetQueryParam("file_id", fileID), http.MethodGet, apiURL+"/open/folder/get_info")
	if err != nil {
		return FileInfo{}, err
	}
	var result struct {
		apiResponse
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return FileInfo{}, fmt.Errorf("decode 115 file info: %w", err)
	}
	if err := result.err(); err != nil {
		return FileInfo{}, err
	}
	data := result.Data
	if len(data) > 0 && data[0] == '[' {
		var entries []json.RawMessage
		if err := json.Unmarshal(data, &entries); err != nil {
			return FileInfo{}, fmt.Errorf("decode 115 file info list: %w", err)
		}
		if len(entries) == 0 {
			return FileInfo{}, ErrNotFound
		}
		if len(entries) != 1 {
			return FileInfo{}, fmt.Errorf("115 returned multiple objects for one file")
		}
		data = entries[0]
	}
	var wire struct {
		ID       string `json:"file_id"`
		Name     string `json:"file_name"`
		Category string `json:"file_category"`
		Size     int64  `json:"size_byte"`
		PickCode string `json:"pick_code"`
		SHA1     string `json:"sha1"`
		Paths    []struct {
			ID   string `json:"file_id"`
			Name string `json:"file_name"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return FileInfo{}, fmt.Errorf("decode 115 file info data: %w", err)
	}
	if wire.ID != fileID || (wire.Category != "0" && wire.Category != "1") {
		return FileInfo{}, fmt.Errorf("115 returned invalid file info for %s", fileID)
	}
	info := FileInfo{File: File{ID: wire.ID, Name: wire.Name, ParentID: "0",
		IsDirectory: wire.Category == "0", Size: wire.Size, PickCode: wire.PickCode, SHA1: wire.SHA1},
		Path: make([]Directory, len(wire.Paths)),
	}
	for i, dir := range wire.Paths {
		info.Path[i] = Directory{ID: dir.ID, Name: dir.Name}
		info.ParentID = dir.ID
	}
	return info, nil
}

// ReadMetadata reads a small sidecar, never the video itself. The signed URL
// request uses the same User-Agent as downurl and carries no access token.
func (client *Client) ReadMetadata(ctx context.Context, accessToken, pickCode string, limit int64) ([]byte, error) {
	const userAgent = "Miyabi/1.0"
	response, err := client.request(client.http.R().SetContext(ctx).SetAuthToken(accessToken).
		SetHeader("User-Agent", userAgent).SetFormData(map[string]string{"pick_code": pickCode}),
		http.MethodPost, apiURL+"/open/ufile/downurl")
	if err != nil {
		return nil, err
	}
	var result struct {
		apiResponse
		Data map[string]struct {
			URL struct {
				URL string `json:"url"`
			} `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return nil, fmt.Errorf("decode 115 download URL: %w", err)
	}
	if err := result.err(); err != nil {
		return nil, err
	}
	if len(result.Data) != 1 {
		return nil, fmt.Errorf("115 returned no unique metadata download URL")
	}
	var downloadURL string
	for _, item := range result.Data {
		downloadURL = item.URL.URL
	}
	if downloadURL == "" {
		return nil, fmt.Errorf("115 metadata download URL is empty")
	}
	if err := client.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	download, err := client.http.R().SetContext(ctx).SetHeader("User-Agent", userAgent).
		SetDoNotParseResponse(true).Get(downloadURL)
	if err != nil {
		return nil, err
	}
	defer download.RawBody().Close()
	if !download.IsSuccess() {
		return nil, fmt.Errorf("115 metadata download returned HTTP %d", download.StatusCode())
	}
	body, err := io.ReadAll(io.LimitReader(download.RawBody(), limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("115 metadata exceeds %d bytes", limit)
	}
	return body, nil
}
