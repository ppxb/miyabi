package service

import (
	"slices"
	"strings"

	"github.com/ppxb/miyabi/internal/pan"
)

// Reconciliation only needs names/hashes and which videos share a directory.
// Pick codes, sizes, parent IDs and child directory entries stay page-local.
type observedFile struct {
	metadataSidecar
	videoID string
}

type directoryObservation []observedFile

type scanObservations map[string]directoryObservation

func (observed scanObservations) add(id string, files []pan.File) {
	directory := observed[id]
	fileCount := 0
	for _, entry := range files {
		if !entry.IsDirectory {
			fileCount++
		}
	}
	directory = slices.Grow(directory, fileCount)
	for _, entry := range files {
		if entry.IsDirectory {
			continue
		}
		file := observedFile{metadataSidecar: metadataSidecar{Name: entry.Name, SHA1: entry.SHA1}}
		if isVideo(entry.Name) {
			file.videoID = entry.ID
		}
		directory = append(directory, file)
	}
	observed[id] = directory
}

func (directory directoryObservation) sidecar(name string) (metadataSidecar, bool) {
	for _, entry := range directory {
		if strings.EqualFold(entry.Name, name) {
			return entry.metadataSidecar, true
		}
	}
	return metadataSidecar{}, false
}
