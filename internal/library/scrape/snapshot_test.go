package scrape

import (
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/pan"
)

func TestVideoFingerprintDeterministic(t *testing.T) {
	files1 := []pan.File{
		{ID: "1", ParentID: "p1", Name: "part1.mp4", SHA1: "aaaa", Size: 1000},
		{ID: "2", ParentID: "p1", Name: "part2.mp4", SHA1: "bbbb", Size: 2000},
	}
	files2 := []pan.File{
		{ID: "2", ParentID: "p1", Name: "part2.mp4", SHA1: "bbbb", Size: 2000},
		{ID: "1", ParentID: "p1", Name: "part1.mp4", SHA1: "aaaa", Size: 1000},
	}
	fp1 := VideoFingerprint(files1)
	fp2 := VideoFingerprint(files2)
	if fp1 != fp2 {
		t.Fatalf("expected fingerprints to match regardless of slice order: %q != %q", fp1, fp2)
	}
}

func TestSnapshotMatches(t *testing.T) {
	video := pan.File{ID: "v1", ParentID: "dir-1", Name: "ABP-001.mp4", SHA1: "sha-video", Size: 1 << 30}
	nfoFile := pan.File{Name: "ABP-001.nfo", SHA1: strings.Repeat("1", 40)}
	poster := pan.File{Name: "poster.jpg", SHA1: strings.Repeat("2", 40)}
	fanart := pan.File{Name: "fanart.jpg", SHA1: strings.Repeat("3", 40)}

	snapshot := domain.MetadataSnapshot{
		Videos: VideoFingerprint([]pan.File{video}),
		Directories: []domain.DirectorySnapshot{
			NewDirectorySnapshot("dir-1", nfoFile, poster, fanart),
		},
	}

	record := &ent.Movie{
		ID:               1,
		Code:             "ABP-001",
		MetadataSnapshot: &snapshot,
		Edges: ent.MovieEdges{
			Files: []*ent.File{
				{FileID: video.ID, ParentID: video.ParentID, Name: video.Name, Sha1: video.SHA1, Size: video.Size},
			},
		},
	}

	observations := DirectoryObservations{
		"dir-1": ObservedDirectory{
			{Sidecar: domain.Sidecar{Name: video.Name, SHA1: video.SHA1}, VideoID: video.ID},
			{Sidecar: domain.Sidecar{Name: nfoFile.Name, SHA1: nfoFile.SHA1}},
			{Sidecar: domain.Sidecar{Name: poster.Name, SHA1: poster.SHA1}},
			{Sidecar: domain.Sidecar{Name: fanart.Name, SHA1: fanart.SHA1}},
		},
	}

	if !SnapshotMatches(record, domain.LibrarySource{}, observations) {
		t.Fatal("expected snapshot to match unchanged observations")
	}

	// Change NFO hash
	changedObservations := DirectoryObservations{
		"dir-1": ObservedDirectory{
			{Sidecar: domain.Sidecar{Name: video.Name, SHA1: video.SHA1}, VideoID: video.ID},
			{Sidecar: domain.Sidecar{Name: nfoFile.Name, SHA1: strings.Repeat("9", 40)}},
			{Sidecar: domain.Sidecar{Name: poster.Name, SHA1: poster.SHA1}},
			{Sidecar: domain.Sidecar{Name: fanart.Name, SHA1: fanart.SHA1}},
		},
	}
	if SnapshotMatches(record, domain.LibrarySource{}, changedObservations) {
		t.Fatal("expected snapshot not to match when NFO SHA1 changes")
	}
}

func TestSnapshotMatchesLocalExport(t *testing.T) {
	video := pan.File{ID: "v1", ParentID: "dir-1", Name: "ABP-001.mp4", SHA1: "sha-video", Size: 1 << 30}
	snapshot := domain.MetadataSnapshot{
		Videos:      VideoFingerprint([]pan.File{video}),
		LocalExport: true,
	}

	record := &ent.Movie{
		ID:               1,
		Code:             "ABP-001",
		MetadataSnapshot: &snapshot,
		Edges: ent.MovieEdges{
			Files: []*ent.File{
				{FileID: video.ID, ParentID: video.ParentID, Name: video.Name, Sha1: video.SHA1, Size: video.Size},
			},
		},
	}

	// LocalExport should match even if 115 directory has no sidecars at all
	observations := DirectoryObservations{
		"dir-1": ObservedDirectory{
			{Sidecar: domain.Sidecar{Name: video.Name, SHA1: video.SHA1}, VideoID: video.ID},
		},
	}
	if !SnapshotMatches(record, domain.LibrarySource{}, observations) {
		t.Fatal("expected LocalExport snapshot to match unchanged video even without sidecars in 115")
	}

	// If video changed, LocalExport snapshot should NOT match
	modifiedRecord := &ent.Movie{
		ID:               1,
		Code:             "ABP-001",
		MetadataSnapshot: &snapshot,
		Edges: ent.MovieEdges{
			Files: []*ent.File{
				{FileID: video.ID, ParentID: video.ParentID, Name: video.Name, Sha1: "changed-sha", Size: video.Size},
			},
		},
	}
	if SnapshotMatches(modifiedRecord, domain.LibrarySource{}, observations) {
		t.Fatal("expected LocalExport snapshot not to match when video files changed")
	}
}
