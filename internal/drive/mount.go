package drive

import (
	"slices"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/pan"
)

// WithinSource checks whether a file info record belongs inside the mounted library source.
func WithinSource(info pan.FileInfo, source domain.LibrarySource) bool {
	return info.ID == source.Directory.ID || source.Directory.ID == "0" ||
		slices.ContainsFunc(info.Path, func(dir pan.Directory) bool { return dir.ID == source.Directory.ID })
}

// directoryPath builds a root-anchored path string from 115 path segments,
// skipping the root folder (ID "0").
func directoryPath(segments []pan.Directory) string {
	names := make([]string, 0, len(segments))
	for _, directory := range segments {
		if directory.ID != "0" && directory.Name != "" {
			names = append(names, directory.Name)
		}
	}
	return "/" + strings.Join(names, "/")
}

// FilePath builds a root-anchored path string from 115 path segments and a file name.
func FilePath(segments []pan.Directory, name string) string {
	dir := directoryPath(segments)
	if dir == "/" {
		return "/" + name
	}
	return dir + "/" + name
}
