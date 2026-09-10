package database

import (
	"encoding/json"
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
	job := store.Client.Task.Create().SetType("scan").SetPayload(json.RawMessage(`{"fixture":"preserved"}`)).SaveX(t.Context())
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
	if got := store.Client.Task.GetX(t.Context(), job.ID); string(got.Payload) != `{"fixture":"preserved"}` {
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

func TestMovieWatchStateMigratesAndSurvivesReopen(t *testing.T) {
	directory := t.TempDir()
	ctx := t.Context()
	store, err := Open(ctx, directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if store != nil {
			_ = store.Close()
		}
	})
	film := store.Client.Movie.Create().SetCode("ABP-001").SetTitle("Existing title").SaveX(ctx)
	video := store.Client.File.Create().SetFileID("video").SetName("ABP-001.mp4").SetSize(1 << 30).
		SetAccountID("100").SetRootID("10").SetMovie(film).SaveX(ctx)
	// An existing installation has movies and files but no watch-state column.
	if _, err := store.db.ExecContext(ctx, "ALTER TABLE movies DROP COLUMN watched"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, directory)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Client.Movie.GetX(ctx, film.ID); got.Watched || got.Title != film.Title {
		t.Fatalf("migration lost movie data or invented watch history: %+v", got)
	}
	if got := store.Client.File.GetX(ctx, video.ID); got.MovieID == nil || *got.MovieID != film.ID {
		t.Fatal("watch-state migration changed the file association")
	}
	store.Client.Movie.UpdateOneID(film.ID).SetWatched(true).ExecX(ctx)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, directory)
	if err != nil {
		t.Fatal(err)
	}
	if !store.Client.Movie.GetX(ctx, film.ID).Watched {
		t.Fatal("restart discarded saved watch state")
	}
}

func TestStoredTaskJSONSurvivesReopenWithoutReencoding(t *testing.T) {
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
	job := store.Client.Task.Create().SetType("scan").SaveX(t.Context())
	// This is the existing on-disk JSON contract, written without using the new
	// Go payload type. IDs must remain numbers and optional keys stay absent.
	legacy := `{"source":{"account_id":"100","directory":{"id":"10","path":"/Movies"}},"scan":{"stage":"scanning","files_scanned":7},"offline_task_id":9007199254740993,"future":{"keep":true}}`
	if _, err := store.db.ExecContext(t.Context(), "UPDATE tasks SET payload = ? WHERE id = ?", legacy, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	loaded := store.Client.Task.GetX(t.Context(), job.ID)
	if string(loaded.Payload) != legacy {
		t.Fatalf("stored task was changed or double encoded: %s", loaded.Payload)
	}
	var kind string
	var offlineID int64
	if err := store.db.QueryRowContext(t.Context(),
		"SELECT json_type(payload), json_extract(payload, '$.offline_task_id') FROM tasks WHERE id = ?", job.ID).
		Scan(&kind, &offlineID); err != nil {
		t.Fatal(err)
	}
	if kind != "object" || offlineID != 9007199254740993 {
		t.Fatalf("stored task no longer supports JSON identity queries: %s %d", kind, offlineID)
	}
}

func TestOfflineHistoryIndexesSurviveReopenAndSupportGrouping(t *testing.T) {
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
	job := store.Client.Task.Create().SetType("offline").
		SetPayload(json.RawMessage(`{"account_id":"100","directory_id":"10","javdb_id":"movie","hash":"fixture"}`)).SaveX(t.Context())
	// Upgrade a database populated before the expression indexes existed.
	for _, name := range []string{"task_offline_source_history", "task_offline_movie_history"} {
		if _, err := store.db.ExecContext(t.Context(), "DROP INDEX "+name); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		store, err = Open(t.Context(), directory)
		if err != nil {
			t.Fatal(err)
		}
		if got := store.Client.Task.GetX(t.Context(), job.ID); string(got.Payload) != string(job.Payload) || got.Status != job.Status {
			t.Fatal("history index migration changed a task")
		}
		for _, query := range []struct{ field, value, index string }{
			{"directory_id", "10", "task_offline_source_history"},
			{"javdb_id", "movie", "task_offline_movie_history"},
		} {
			rows, err := store.db.QueryContext(t.Context(),
				"EXPLAIN QUERY PLAN SELECT MAX(id) FROM tasks WHERE type = ? AND json_extract(payload, '$.account_id') = ? AND json_extract(payload, '$."+query.field+"') = ? GROUP BY json_type(payload, '$.hash'), json_extract(payload, '$.hash')",
				"offline", "100", query.value)
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
			plan := strings.Join(details, "\n")
			if err != nil || !strings.Contains(plan, query.index) || strings.Contains(plan, "TEMP B-TREE FOR GROUP BY") {
				t.Fatalf("history query did not use its ordered scope index: %s, %v", plan, err)
			}
		}
	}
}
