package pan

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
	type fileListWire struct {
		apiResponse
		CID   json.Number `json:"cid"`
		Count json.Number `json:"count"`
		Data  []struct {
			ID       string      `json:"fid"`
			ParentID string      `json:"pid"`
			Name     string      `json:"fn"`
			Category string      `json:"fc"`
			Size     json.Number `json:"fs"`
			PickCode string      `json:"pc"`
			SHA1     string      `json:"sha1"`
		} `json:"data"`
		Path []struct {
			ID   json.Number `json:"cid"`
			Name string      `json:"name"`
		} `json:"path"`
	}
	result, err := apiRequest[fileListWire](
		client,
		client.http.R().SetContext(ctx).SetAuthToken(accessToken).SetQueryParams(map[string]string{
			"cid": directoryID, "offset": strconv.Itoa(offset), "limit": strconv.Itoa(limit),
			"show_dir": "1", "stdir": "1", "cur": "1", "o": "file_name", "asc": "1",
		}),
		http.MethodGet,
		apiURL+"/open/ufile/files",
		"file list",
	)
	if err != nil {
		return FilePage{}, err
	}
	if result.CID.String() != directoryID || len(result.Path) == 0 || result.Path[len(result.Path)-1].ID.String() != directoryID {
		return FilePage{}, fmt.Errorf("115 returned a different directory than requested")
	}
	total, err := result.Count.Int64()
	if err != nil && result.Count != "" {
		return FilePage{}, fmt.Errorf("decode 115 file count: %w", err)
	}
	page := FilePage{
		Files: make([]File, len(result.Data)), Path: make([]Directory, len(result.Path)),
		Total: int(total), HasMore: int64(offset+len(result.Data)) < total,
	}
	for index, file := range result.Data {
		if file.Category != "0" && file.Category != "1" {
			return FilePage{}, fmt.Errorf("115 returned unknown file category %q", file.Category)
		}
		fileSize, err := file.Size.Int64()
		if err != nil && file.Size != "" {
			return FilePage{}, fmt.Errorf("decode 115 file size: %w", err)
		}
		page.Files[index] = File{
			ID: file.ID, ParentID: file.ParentID, Name: file.Name,
			IsDirectory: file.Category == "0", Size: fileSize, PickCode: file.PickCode, SHA1: file.SHA1,
		}
	}
	for index, directory := range result.Path {
		page.Path[index] = Directory{ID: directory.ID.String(), Name: directory.Name}
	}
	return page, nil
}

// SHA1 computes the uppercase hexadecimal SHA-1 digest for a byte slice.
func SHA1(body []byte) string {
	sum := sha1.Sum(body)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

