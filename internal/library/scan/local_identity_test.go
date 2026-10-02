package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
)

func TestLocalFilesInDifferentRootsKeepSeparateIdentities(t *testing.T) {
	for _, extension := range []string{".mp4", ".strm"} {
		t.Run(extension, func(t *testing.T) {
			scanner := localScannerFixture(t)
			ctx := t.Context()
			firstRoot, secondRoot := t.TempDir(), t.TempDir()
			name := "TEST-001" + extension
			writeLocalFile(t, firstRoot, name, []byte("https://example.com/first"))
			writeLocalFile(t, secondRoot, name, []byte("https://example.com/second-version"))
			if _, err := scanner.Scan(ctx, firstRoot); err != nil {
				t.Fatal(err)
			}
			first := scanner.db.File.Query().OnlyX(ctx)
			corrected := scanner.db.Movie.UpdateOneID(*first.MovieID).SetCode("FIX-999").SetManualCode("FIX-999").
				SetTitle("manually corrected").SaveX(ctx)
			if _, err := scanner.Scan(ctx, secondRoot); err != nil {
				t.Fatal(err)
			}
			if count := scanner.db.File.Query().CountX(ctx); count != 2 {
				t.Fatalf("identical relative paths in different roots produced %d file rows", count)
			}
			second := scanner.db.File.Query().Where(file.IDNEQ(first.ID)).OnlyX(ctx)
			film := scanner.db.Movie.Query().Where(movie.CodeEQ("TEST-001")).OnlyX(ctx)
			if first.FileID == second.FileID || second.MovieID == nil || *second.MovieID != film.ID || film.ID == corrected.ID {
				t.Fatal("second root reused the first root's corrected file association")
			}
			if second.Size != int64(len("https://example.com/second-version")) || second.Path != name {
				t.Fatalf("second root did not retain its file data: %+v", second)
			}
			// File facts must refresh even when manual movie metadata is protected.
			scanner.db.File.UpdateOneID(first.ID).SetName("stale-name").SetSize(0).SetPath("stale-path").ExecX(ctx)
			writeLocalFile(t, firstRoot, name, []byte("updated file content"))
			if _, err := scanner.Scan(ctx, firstRoot); err != nil {
				t.Fatal(err)
			}
			updated := scanner.db.File.GetX(ctx, first.ID)
			if updated.FileID != first.FileID || updated.Name != name || updated.Path != name || updated.Size != int64(len("updated file content")) || *updated.MovieID != corrected.ID {
				t.Fatalf("rescan lost identity or retained stale file facts: %+v", updated)
			}
			if got := scanner.db.Movie.GetX(ctx, corrected.ID); got.Code != corrected.Code || got.Title != corrected.Title || got.ManualCode != corrected.ManualCode {
				t.Fatal("rescan replaced manual movie metadata")
			}
			if scanner.db.File.Query().CountX(ctx) != 2 || scanner.db.Movie.Query().CountX(ctx) != 2 {
				t.Fatal("rescan duplicated files or movies")
			}
		})
	}
}

func TestLocalScanNormalizesRelativeAndAbsoluteRoots(t *testing.T) {
	scanner := localScannerFixture(t)
	root := t.TempDir()
	writeLocalFile(t, root, "TEST-001.mp4", []byte("video"))
	t.Chdir(root)
	for _, path := range []string{root, ".", root + string(filepath.Separator) + "."} {
		if _, err := scanner.Scan(t.Context(), path); err != nil {
			t.Fatal(err)
		}
	}
	if scanner.db.File.Query().CountX(t.Context()) != 1 {
		t.Fatal("equivalent root paths duplicated the file")
	}
}

func TestLocalSTRMRescanRefreshesFileFacts(t *testing.T) {
	scanner := localScannerFixture(t)
	root, ctx := t.TempDir(), t.Context()
	name := "TEST-001.strm"
	content := []byte("http://localhost/api/strm/play/remote-1")
	writeLocalFile(t, root, name, content)
	if _, err := scanner.Scan(ctx, root); err != nil {
		t.Fatal(err)
	}
	before := scanner.db.File.Query().OnlyX(ctx)
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0700); err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join("subdir", "TEST-001-part1.strm")
	if err := os.Rename(filepath.Join(root, name), filepath.Join(root, rel)); err != nil {
		t.Fatal(err)
	}
	content = append(content, '\n')
	writeLocalFile(t, root, rel, content)
	if _, err := scanner.Scan(ctx, root); err != nil {
		t.Fatal(err)
	}
	after := scanner.db.File.Query().OnlyX(ctx)
	if after.ID != before.ID || after.FileID != "remote-1" || after.Path != rel || after.Name != filepath.Base(rel) || after.Size != int64(len(content)) {
		t.Fatalf("local STRM facts were not refreshed: %+v", after)
	}
}
