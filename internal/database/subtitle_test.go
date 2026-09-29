package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenRemovesPanSubtitleStorageAndPreservesOnlineExports(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if store != nil {
			_ = store.Close()
		}
	})
	file := filepath.Join(dir, "existing.srt")
	if err := os.WriteFile(file, []byte("existing subtitle"), 0o644); err != nil {
		t.Fatal(err)
	}
	movie := store.Client.Movie.Create().SetCode("ABP-123").SaveX(ctx)
	online := store.Client.Subtitle.Create().SetMovieID(movie.ID).SetName("online.srt").
		SetSource("SubtitleCat").SetSourceURL("https://example.com/subtitle").SetStoragePath(file).SaveX(ctx)
	store.Client.Subtitle.Create().SetMovieID(movie.ID).SetName("exported.srt").
		SetSource("115").SetStoragePath(file).SaveX(ctx)
	legacy := store.Client.Subtitle.Create().SetMovieID(movie.ID).SetName("pending.srt").SaveX(ctx)
	for _, statement := range []string{
		"ALTER TABLE subtitles ADD COLUMN file_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE subtitles ADD COLUMN pick_code TEXT NOT NULL DEFAULT ''",
		"CREATE INDEX subtitle_file_id ON subtitles(file_id)",
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE subtitles SET file_id = 'file', pick_code = 'pick' WHERE id = ?", legacy.ID); err != nil {
		t.Fatal(err)
	}
	setMigrationVersion(t, store, len(migrations)-1)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		store, err = Open(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		records := store.Client.Subtitle.Query().AllX(ctx)
		if len(records) != 1 || records[0].ID != online.ID || records[0].StoragePath != file || records[0].SourceURL != online.SourceURL {
			t.Fatalf("unexpected remaining subtitles: %+v", records)
		}
		var count int
		if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pragma_table_info('subtitles') WHERE name IN ('file_id', 'pick_code')").Scan(&count); err != nil || count != 0 {
			t.Fatalf("legacy columns = %d, error = %v", count, err)
		}
		if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'subtitle_file_id'").Scan(&count); err != nil || count != 0 {
			t.Fatalf("legacy indexes = %d, error = %v", count, err)
		}
		if got := migrationVersion(t, store.db); got != len(migrations) {
			t.Fatalf("migration version = %d", got)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		store = nil
	}
	if body, err := os.ReadFile(file); err != nil || string(body) != "existing subtitle" {
		t.Fatalf("exported file changed: %q, %v", body, err)
	}
}
