package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
)

// A .strm URL like /api/strm/play/115/1001 carries the real id in a later path
// segment. Two such files must stay distinct movies; before the fix both parsed
// to the same leading "115", one file record overwrote the other, and the movie
// left without a file was hidden from the library listing.
func TestLocalImportKeepsMoviesWhenSTRMIDsShareLeadingSegment(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	dir := filepath.Join(root, "abc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("DEMO-002.strm", "https://example.invalid/api/strm/play/115/1001")
	write("DEMO-002.nfo", `<movie><num>DEMO-002</num><title>示例影片一</title></movie>`)
	write("DEMO-004.strm", "https://example.invalid/api/strm/play/115/1002")
	write("DEMO-004.nfo", `<movie><num>DEMO-004</num><title>示例影片二</title></movie>`)

	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	scanner := NewLocalScanner(store.Client, nil)
	res, err := scanner.Scan(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if res.MediaFiles != 2 || res.MoviesAdded != 2 {
		t.Fatalf("unexpected scan result: %+v", res)
	}
	if got := store.Client.File.Query().CountX(ctx); got != 2 {
		t.Fatalf("file count = %d, want 2", got)
	}
	for _, code := range []string{"DEMO-002", "DEMO-004"} {
		film := store.Client.Movie.Query().Where(movie.CodeEQ(code)).OnlyX(ctx)
		if n := store.Client.File.Query().Where(file.MovieIDEQ(film.ID)).CountX(ctx); n != 1 {
			t.Fatalf("movie %s has %d files, want 1", code, n)
		}
	}
}

// Emby-style stem-suffixed PNG artwork (<stem>-poster.png) must be picked up,
// not only .jpg variants and bare poster.png.
func TestFindMatchingArtworkMatchesStemSuffixedPNG(t *testing.T) {
	dir := t.TempDir()
	poster := filepath.Join(dir, "DEMO-002-poster.png")
	fanart := filepath.Join(dir, "DEMO-002-fanart.png")
	for _, p := range []string{poster, fanart} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gotPoster, gotFanart := findMatchingArtwork(dir, "DEMO-002", []string{poster, fanart})
	if gotPoster != poster {
		t.Fatalf("poster = %q, want %q", gotPoster, poster)
	}
	if gotFanart != fanart {
		t.Fatalf("fanart = %q, want %q", gotFanart, fanart)
	}
}
