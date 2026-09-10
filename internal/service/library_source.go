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
	state, err := service.drive.sourceState(source, version)
	if err != nil {
		return pan.FileInfo{}, err
	}
	info, err := withPanSourceToken(ctx, service.drive, state, func(token string) (pan.FileInfo, error) {
		return service.drive.client.Info(ctx, token, id)
	})
	if err != nil {
		return pan.FileInfo{}, err
	}
	if err := service.checkScanSource(source, version); err != nil {
		return pan.FileInfo{}, err
	}
	if !withinSource(info, source) {
		return pan.FileInfo{}, fmt.Errorf("下载资源已移出媒体目录")
	}
	return info, nil
}

func (service *LibraryService) readSidecar(ctx context.Context, source LibrarySource, version uint64, entry pan.File, limit int64) ([]byte, error) {
	state, err := service.drive.sourceState(source, version)
	if err != nil {
		return nil, err
	}
	body, err := withPanSourceToken(ctx, service.drive, state, func(token string) ([]byte, error) {
		return service.drive.client.ReadMetadata(ctx, token, entry.PickCode, limit)
	})
	if err != nil {
		return nil, err
	}
	if err := service.checkScanSource(source, version); err != nil {
		return nil, err
	}
	return body, nil
}

func (service *LibraryService) directoryEntries(ctx context.Context, source LibrarySource, version uint64, id string) ([]pan.File, error) {
	var files []pan.File
	err := walkFilePages(ctx, func(offset int) (pan.FilePage, error) {
		return service.scanPage(ctx, source, version, id, offset)
	}, func(page pan.FilePage) (bool, error) {
		files = append(files, page.Files...)
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func sidecarByName(files []pan.File, name string) (pan.File, bool) {
	for _, entry := range files {
		if !entry.IsDirectory && strings.EqualFold(entry.Name, name) {
			return entry, true
		}
	}
	return pan.File{}, false
}
