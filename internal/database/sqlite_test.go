package database

import (
	"strings"
	"testing"
)

func TestExistingLibraryGainsIndexesWithoutChangingRecords(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if store != nil {
			_ = store.Close()
		}
	})
	movie := store.Client.Movie.Create().SetCode("ABP-001").SetTitle("Preserved title").SaveX(t.Context())
	file := store.Client.File.Create().SetFileID("video").SetName("ABP-001.mp4").SetSize(1024).
		SetAccountID("100").SetRootID("10").SetMovie(movie).SaveX(t.Context())
	job := store.Client.Task.Create().SetType("scan").SetPayload(map[string]any{"fixture": "preserved"}).SaveX(t.Context())
	// Model the previous schema while keeping populated application tables.
	for _, index := range []string{"file_movie_files_account_id_root_id", "task_type"} {
		if _, err := store.db.ExecContext(t.Context(), "DROP INDEX "+index); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Client.Movie.GetX(t.Context(), movie.ID); got.Title != movie.Title {
		t.Fatal("migration changed movie metadata")
	}
	if got := store.Client.File.GetX(t.Context(), file.ID); got.MovieID == nil || *got.MovieID != movie.ID || got.FileID != file.FileID {
		t.Fatal("migration changed a file association")
	}
	if got := store.Client.Task.GetX(t.Context(), job.ID); got.Payload["fixture"] != "preserved" {
		t.Fatal("migration changed a stored task")
	}
	for _, check := range []struct{ query, index string }{
		{"SELECT id FROM files WHERE movie_files=1", "file_movie_files_account_id_root_id"},
		{"SELECT id FROM files WHERE movie_files=1 AND account_id='100' AND root_id='10'", "file_movie_files_account_id_root_id"},
		{"SELECT id FROM tasks WHERE type='scan' ORDER BY id DESC LIMIT 20", "task_type"},
	} {
		rows, err := store.db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+check.query)
		if err != nil {
			t.Fatal(err)
		}
		var details []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			details = append(details, detail)
		}
		err = rows.Err()
		rows.Close()
		if err != nil || !strings.Contains(strings.Join(details, "\n"), "USING COVERING INDEX "+check.index) {
			t.Fatalf("index not used for %s: %v, %v", check.query, details, err)
		}
	}
}
