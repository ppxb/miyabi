package service

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/ppxb/miyabi/internal/pan"
)

func fileInfoPath(info pan.FileInfo) string {
	names := []string{"/"}
	for _, directory := range info.Path {
		if directory.ID != "0" {
			names = append(names, directory.Name)
		}
	}
	names = append(names, info.Name)
	return path.Join(names...)
}

func withinSource(info pan.FileInfo, source LibrarySource) bool {
	return info.ID == source.Directory.ID || source.Directory.ID == "0" ||
		slices.ContainsFunc(info.Path, func(dir pan.Directory) bool { return dir.ID == source.Directory.ID })
}

func (service *LibraryService) sourceInfo(ctx context.Context, source LibrarySource, version uint64, id string) (pan.FileInfo, error) {
	service.drive.mu.Lock()
	defer service.drive.mu.Unlock()
	if err := service.checkScanSource(source, version); err != nil {
		return pan.FileInfo{}, err
	}
	info, err := withPanToken(ctx, service.drive, func(token string) (pan.FileInfo, error) {
		return service.drive.client.Info(ctx, token, id)
	})
	if err != nil {
		return pan.FileInfo{}, err
	}
	if !withinSource(info, source) {
		return pan.FileInfo{}, fmt.Errorf("下载资源已移出媒体目录")
	}
	return info, nil
}

func (service *LibraryService) readSidecar(ctx context.Context, source LibrarySource, version uint64, entry pan.File, limit int64) ([]byte, error) {
	service.drive.mu.Lock()
	defer service.drive.mu.Unlock()
	if err := service.checkScanSource(source, version); err != nil {
		return nil, err
	}
	return withPanToken(ctx, service.drive, func(token string) ([]byte, error) {
		return service.drive.client.ReadMetadata(ctx, token, entry.PickCode, limit)
	})
}

func (service *LibraryService) directoryEntries(ctx context.Context, source LibrarySource, version uint64, id string) ([]pan.File, error) {
	var files []pan.File
	total := -1
	for offset := 0; ; {
		page, err := service.scanPage(ctx, source, version, id, offset)
		if err != nil {
			return nil, err
		}
		if total == -1 {
			total = page.Total
		}
		if total != page.Total || (page.HasMore && len(page.Files) == 0) {
			return nil, fmt.Errorf("115 目录内容在读取期间变化，请重新扫描")
		}
		files = append(files, page.Files...)
		offset += len(page.Files)
		if !page.HasMore {
			if offset != total {
				return nil, fmt.Errorf("115 目录分页不完整")
			}
			return files, nil
		}
	}
}

func sidecarByName(files []pan.File, name string) (pan.File, bool) {
	for _, entry := range files {
		if !entry.IsDirectory && strings.EqualFold(entry.Name, name) {
			return entry, true
		}
	}
	return pan.File{}, false
}
