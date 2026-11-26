package scan

import (
	"testing"

	"github.com/ppxb/miyabi/internal/codeid"
)

func TestPreferredNewMovieCode(t *testing.T) {
	for _, tc := range []struct {
		name  string
		code  string
		codes []string
		want  string
	}{
		{"decorated batch", "259LUXU-1899", []string{"259LUXU-1899", "LUXU-1899"}, "LUXU-1899"},
		{"subtitle suffix", "SSNI-001CH", []string{"SSNI-001CH", "SSNI-001"}, "SSNI-001"},
		{"unrelated code", "LUXU-1898", []string{"LUXU-1898", "LUXU-1899"}, "LUXU-1898"},
		{"single code", "ABP-001", []string{"ABP-001"}, "ABP-001"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := preferredNewMovieCode(tc.code, tc.codes)
			if got != tc.want || !codeid.IsEquivalent(got, tc.code) {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestLocalScanReusesExistingShortCode(t *testing.T) {
	scanner := localScannerFixture(t)
	ctx, root := t.Context(), t.TempDir()
	existing := scanner.db.Movie.Create().SetCode("LUXU-1899").SaveX(ctx)
	writeLocalFile(t, root, "259LUXU-1899.mp4", []byte("video"))

	result, err := scanner.Scan(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.MoviesAdded != 0 || result.MediaFiles != 1 {
		t.Fatalf("result=%+v", result)
	}
	records := scanner.db.Movie.Query().AllX(ctx)
	if len(records) != 1 || records[0].ID != existing.ID || records[0].Code != "LUXU-1899" {
		t.Fatalf("movies=%+v", records)
	}
	entry := scanner.db.File.Query().OnlyX(ctx)
	if entry.MovieID == nil || *entry.MovieID != existing.ID {
		t.Fatalf("file not bound to the existing movie: %+v", entry)
	}
}

func TestLocalScanStoresShortestEquivalentCode(t *testing.T) {
	scanner := localScannerFixture(t)
	ctx, root := t.Context(), t.TempDir()
	// WalkDir visits 259LUXU-1899.mp4 before LUXU-1899.mp4, so first-seen
	// creation would store the decorated spelling.
	writeLocalFile(t, root, "259LUXU-1899.mp4", []byte("video"))
	writeLocalFile(t, root, "LUXU-1899.mp4", []byte("video"))

	result, err := scanner.Scan(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.MediaFiles != 2 || result.MoviesAdded != 1 {
		t.Fatalf("result=%+v", result)
	}
	records := scanner.db.Movie.Query().AllX(ctx)
	if len(records) != 1 || records[0].Code != "LUXU-1899" {
		t.Fatalf("movies=%+v", records)
	}
	files := scanner.db.File.Query().AllX(ctx)
	if len(files) != 2 {
		t.Fatalf("files=%d", len(files))
	}
	for _, entry := range files {
		if entry.MovieID == nil || *entry.MovieID != records[0].ID {
			t.Fatalf("file %s not bound to the shared movie", entry.Name)
		}
	}
}
