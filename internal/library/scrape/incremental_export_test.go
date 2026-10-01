package scrape

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/ent"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/nfo"
)

func TestIncrementalExportOnlyWritesChangedFiles(t *testing.T) {
	files := []string{"ABP-123.nfo", "poster.jpg", "fanart.jpg", "ABP-123-cd1.strm", "ABP-123-cd2.strm"}
	for _, scenario := range []string{"unchanged", "fanart.jpg", "poster.jpg", "ABP-123.nfo", "ABP-123-cd2.strm", "corrupt poster", "metadata", "configuration"} {
		t.Run(scenario, func(t *testing.T) {
			images, err := mediaimage.NewCache(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
			art, err := images.Restore(png, png)
			if err != nil {
				t.Fatal(err)
			}
			doc := &nfo.Movie{Code: "ABP-123", Title: "Saved title"}
			record := &ent.Movie{Code: doc.Code, Metadata: doc, Poster: &art.Poster, Fanarts: []string{art.Fanart},
				Edges: ent.MovieEdges{Files: []*ent.File{{FileID: "one", Name: "ABP-123-CD1.mp4"}, {FileID: "two", Name: "ABP-123-CD2.mp4"}}}}
			root := t.TempDir()
			host, token := "http://localhost", "old-token"
			if changed, err := ExportLocalMovie(root, host, token, record, images); err != nil || !changed {
				t.Fatalf("initial export: %v %v", changed, err)
			}
			dir := EmbyMovieDir(root, doc.Code)
			stamp := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			for _, name := range files {
				if err := os.Chtimes(filepath.Join(dir, name), stamp, stamp); err != nil {
					t.Fatal(err)
				}
			}
			var changedFiles []string
			switch scenario {
			case "unchanged":
			case "metadata":
				doc.Title = "Updated title"
				changedFiles = []string{"ABP-123.nfo"}
			case "configuration":
				host, token = "http://new-host", "new-token"
				changedFiles = files[3:]
			case "corrupt poster":
				if err := os.WriteFile(filepath.Join(dir, "poster.jpg"), []byte("incomplete"), 0o644); err != nil {
					t.Fatal(err)
				}
				changedFiles = []string{"poster.jpg"}
			default:
				if err := os.Remove(filepath.Join(dir, scenario)); err != nil {
					t.Fatal(err)
				}
				changedFiles = []string{scenario}
			}
			changed, err := ExportLocalMovie(root, host, token, record, images)
			if err != nil || changed != (len(changedFiles) > 0) {
				t.Fatalf("repair: %v %v", changed, err)
			}
			for _, name := range files {
				info, err := os.Stat(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				if info.ModTime().Equal(stamp) == slices.Contains(changedFiles, name) {
					t.Fatalf("unexpected write: %s", name)
				}
			}
			if changed, err := ExportLocalMovie(root, host, token, record, images); err != nil || changed {
				t.Fatalf("second scan wrote files: %v %v", changed, err)
			}
		})
	}
}
