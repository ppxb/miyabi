package pan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type Directory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type File struct {
	ID          string `json:"id"`
	ParentID    string `json:"parent_id"`
	Name        string `json:"name"`
	IsDirectory bool   `json:"is_directory"`
	Size        int64  `json:"size"`
	PickCode    string `json:"pick_code"`
	SHA1        string `json:"sha1"`
}

type FilePage struct {
	Files   []File      `json:"files"`
	Path    []Directory `json:"path"`
	Total   int         `json:"total"`
	HasMore bool        `json:"has_more"`
}

func (client *Client) List(ctx context.Context, accessToken, directoryID string, offset, limit int) (FilePage, error) {
	response, err := client.request(
		client.http.R().SetContext(ctx).SetAuthToken(accessToken).SetQueryParams(map[string]string{
			"cid": directoryID, "offset": strconv.Itoa(offset), "limit": strconv.Itoa(limit),
			"show_dir": "1", "stdir": "1", "cur": "1", "o": "file_name", "asc": "1",
		}),
		http.MethodGet, apiURL+"/open/ufile/files",
	)
	if err != nil {
		return FilePage{}, err
	}
	var result struct {
		apiResponse
		CID   json.Number `json:"cid"`
		Count int         `json:"count"`
		Data  []struct {
			ID       string `json:"fid"`
			ParentID string `json:"pid"`
			Name     string `json:"fn"`
			Category string `json:"fc"`
			Size     int64  `json:"fs"`
			PickCode string `json:"pc"`
			SHA1     string `json:"sha1"`
		} `json:"data"`
		Path []struct {
			ID   json.Number `json:"cid"`
			Name string      `json:"name"`
		} `json:"path"`
	}
	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return FilePage{}, fmt.Errorf("decode 115 file list: %w", err)
	}
	if err := result.err(); err != nil {
		return FilePage{}, err
	}
	if result.CID.String() != directoryID || len(result.Path) == 0 || result.Path[len(result.Path)-1].ID.String() != directoryID {
		return FilePage{}, fmt.Errorf("115 returned a different directory than requested")
	}
	page := FilePage{
		Files: make([]File, len(result.Data)), Path: make([]Directory, len(result.Path)),
		Total: result.Count, HasMore: offset+len(result.Data) < result.Count,
	}
	for index, file := range result.Data {
		if file.Category != "0" && file.Category != "1" {
			return FilePage{}, fmt.Errorf("115 returned unknown file category %q", file.Category)
		}
		page.Files[index] = File{
			ID: file.ID, ParentID: file.ParentID, Name: file.Name,
			IsDirectory: file.Category == "0", Size: file.Size, PickCode: file.PickCode, SHA1: file.SHA1,
		}
	}
	for index, directory := range result.Path {
		page.Path[index] = Directory{ID: directory.ID.String(), Name: directory.Name}
	}
	return page, nil
}
