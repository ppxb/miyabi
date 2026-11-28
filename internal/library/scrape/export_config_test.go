package scrape

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/export"
	"github.com/ppxb/miyabi/internal/nfo"
)

func TestLocalExportRejectsMissingURLWithoutChangingSidecars(t *testing.T) {
	root := t.TempDir()
	doc := &nfo.Movie{Code: "ABP-123", Title: "Updated title"}
	record := &ent.Movie{Code: doc.Code, Metadata: doc,
		Edges: ent.MovieEdges{Files: []*ent.File{{FileID: "video-1", Name: "ABP-123.mp4"}}}}
	dir := export.EmbyMovieDir(root, doc.Code)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	files := []string{"ABP-123.nfo", "ABP-123.strm", "poster.jpg", "fanart.jpg"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("original "+name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	changed, err := ExportLocalMovie(t.Context(), nil, root, "", "", record, nil)
	if err == nil || changed {
		t.Fatalf("incomplete configuration exported: changed = %t, error = %v", changed, err)
	}
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(data) != "original "+name {
			t.Fatalf("existing %s changed: %q, %v", name, data, err)
		}
	}
}
