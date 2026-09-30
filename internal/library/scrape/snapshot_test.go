package scrape

import (
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	mediaimage "github.com/ppxb/miyabi/internal/image"
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
	source := domain.LibrarySource{AccountID: "account", Directory: domain.LibraryDirectory{ID: "root"}}
	for _, tc := range []struct {
		name   string
		change func(*ent.Movie)
		want   bool
	}{
		{name: "unchanged", want: true},
		{name: "missing snapshot", change: func(m *ent.Movie) { m.MetadataSnapshot = nil }},
		{name: "outdated poster", change: func(m *ent.Movie) { m.MetadataSnapshot.PosterVersion = 0 }},
		{name: "other account", change: func(m *ent.Movie) { m.MetadataSnapshot.AccountID = "other" }},
		{name: "other root", change: func(m *ent.Movie) { m.MetadataSnapshot.DirectoryID = "other" }},
		{name: "moved video", change: func(m *ent.Movie) { m.Edges.Files[0].ParentID = "other" }},
		{name: "renamed video", change: func(m *ent.Movie) { m.Edges.Files[0].Name = "renamed.mp4" }},
		{name: "changed content", change: func(m *ent.Movie) { m.Edges.Files[0].Sha1 = "changed" }},
		{name: "changed size", change: func(m *ent.Movie) { m.Edges.Files[0].Size++ }},
		{name: "removed video", change: func(m *ent.Movie) { m.Edges.Files = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := &ent.Movie{
				MetadataSnapshot: &domain.MetadataSnapshot{AccountID: source.AccountID, DirectoryID: source.Directory.ID, Videos: VideoFingerprint([]pan.File{video}), PosterVersion: mediaimage.PosterVersion},
				Edges:            ent.MovieEdges{Files: []*ent.File{{FileID: video.ID, ParentID: video.ParentID, Name: video.Name, Sha1: video.SHA1, Size: video.Size}}},
			}
			if tc.change != nil {
				tc.change(record)
			}
			if got := SnapshotMatches(record, source); got != tc.want {
				t.Fatalf("match = %t, want %t", got, tc.want)
			}
		})
	}
}
