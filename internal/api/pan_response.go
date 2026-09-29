package api

import (
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/pan"
)

type panAccountResponse struct {
	ID     string                  `json:"id"`
	Name   string                  `json:"name"`
	Avatar string                  `json:"avatar"`
	Level  string                  `json:"level"`
	Space  panAccountSpaceResponse `json:"space"`
}

type panAccountSpaceResponse struct {
	Total     panSpaceAmountResponse `json:"total"`
	Used      panSpaceAmountResponse `json:"used"`
	Remaining panSpaceAmountResponse `json:"remaining"`
}

type panSpaceAmountResponse struct {
	Bytes     int64  `json:"bytes"`
	Formatted string `json:"formatted"`
}

type panAccountStatusResponse struct {
	Connected bool                     `json:"connected"`
	Account   *panAccountResponse      `json:"account,omitempty"`
	Directory *domain.LibraryDirectory `json:"directory,omitempty"`
}

func accountResponse(status drive.AccountStatus) panAccountStatusResponse {
	response := panAccountStatusResponse{Connected: status.Connected, Directory: status.Directory}
	if account := status.Account; account != nil {
		response.Account = &panAccountResponse{
			ID: account.ID, Name: account.Name, Avatar: account.Avatar, Level: account.Level,
			Space: panAccountSpaceResponse{
				Total:     panSpaceAmountResponse{Bytes: account.Space.Total.Bytes, Formatted: account.Space.Total.Formatted},
				Used:      panSpaceAmountResponse{Bytes: account.Space.Used.Bytes, Formatted: account.Space.Used.Formatted},
				Remaining: panSpaceAmountResponse{Bytes: account.Space.Remaining.Bytes, Formatted: account.Space.Remaining.Formatted},
			},
		}
	}
	return response
}

type panFileResponse struct {
	ID          string `json:"id"`
	ParentID    string `json:"parent_id"`
	Name        string `json:"name"`
	IsDirectory bool   `json:"is_directory"`
	Size        int64  `json:"size"`
	PickCode    string `json:"pick_code"`
	SHA1        string `json:"sha1"`
}

type panDirectoryResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type panFilePageResponse struct {
	Files   []panFileResponse      `json:"files"`
	Path    []panDirectoryResponse `json:"path"`
	Total   int                    `json:"total"`
	HasMore bool                   `json:"has_more"`
}

func filesResponse(page pan.FilePage) panFilePageResponse {
	response := panFilePageResponse{Total: page.Total, HasMore: page.HasMore}
	if page.Files != nil {
		response.Files = make([]panFileResponse, len(page.Files))
	}
	for i, file := range page.Files {
		response.Files[i] = panFileResponse{
			ID: file.ID, ParentID: file.ParentID, Name: file.Name, IsDirectory: file.IsDirectory,
			Size: file.Size, PickCode: file.PickCode, SHA1: file.SHA1,
		}
	}
	if page.Path != nil {
		response.Path = make([]panDirectoryResponse, len(page.Path))
	}
	for i, directory := range page.Path {
		response.Path[i] = panDirectoryResponse{ID: directory.ID, Name: directory.Name}
	}
	return response
}
