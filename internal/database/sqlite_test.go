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

func TestPlayerStorageIsDroppedWithoutChangingLibraryRecords(t *testing.T) {
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
	track := store.Client.Subtitle.Create().SetMovie(film).SetName("ABP-001.zh-CN.srt").SetSource("115").
		SetFileID("sub").SetPickCode("pick").SaveX(ctx)
	// Installations with the in-app player stored watch state and player-only subtitle settings.
	for _, statement := range []string{
		"ALTER TABLE movies ADD COLUMN watched bool NOT NULL DEFAULT false",
		"ALTER TABLE subtitles ADD COLUMN display_name text NOT NULL DEFAULT '简体中文'",
		"ALTER TABLE subtitles ADD COLUMN offset_ms integer NOT NULL DEFAULT 0",
		"ALTER TABLE subtitles ADD COLUMN is_default bool NOT NULL DEFAULT false",
		"CREATE INDEX subtitle_movie_id_is_default ON subtitles (movie_id, is_default)",
		`CREATE TABLE watch_histories (id integer PRIMARY KEY AUTOINCREMENT, movie_id integer NOT NULL,
			CONSTRAINT watch_histories_movies_watch_history FOREIGN KEY (movie_id) REFERENCES movies (id) ON DELETE CASCADE)`,
		"INSERT INTO watch_histories (movie_id) VALUES (1)",
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, directory)
	if err != nil {
		t.Fatal(err)
	}
	var leftovers int
	if err := store.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM sqlite_master WHERE name IN ('watch_histories', 'subtitle_movie_id_is_default')) +
		(SELECT COUNT(*) FROM pragma_table_info('movies') WHERE name = 'watched') +
		(SELECT COUNT(*) FROM pragma_table_info('subtitles') WHERE name IN ('display_name', 'offset_ms', 'is_default'))`,
	).Scan(&leftovers); err != nil || leftovers != 0 {
		t.Fatalf("player storage survived migration: %d, %v", leftovers, err)
	}
	if got := store.Client.Movie.GetX(ctx, film.ID); got.Title != film.Title {
		t.Fatalf("migration changed movie metadata: %+v", got)
	}
	if got := store.Client.Subtitle.GetX(ctx, track.ID); got.PickCode != "pick" || got.Name != track.Name {
		t.Fatalf("migration changed a subtitle track: %+v", got)
	}
	// New tracks no longer supply the dropped NOT NULL columns.
	store.Client.Subtitle.Create().SetMovie(film).SetName("ABP-001.zh-TW.srt").ExecX(ctx)
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

func TestMigrateMonitorsToSubscriptions(t *testing.T) {
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

	// Simulate legacy monitors table
	_, err = store.db.ExecContext(t.Context(), `
		CREATE TABLE monitors (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			movie_id TEXT NOT NULL UNIQUE,
			code TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			cover TEXT NOT NULL DEFAULT '',
			release_date TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'waiting',
			hash TEXT NOT NULL DEFAULT '',
			task_id INTEGER,
			next_check_at DATETIME,
			last_checked_at DATETIME,
			checks INTEGER NOT NULL DEFAULT 0,
			error TEXT
		);
		INSERT INTO monitors (id, created_at, updated_at, movie_id, code, title, status, checks)
		VALUES (1, '2026-01-01 00:00:00', '2026-01-01 00:00:00', 'm-001', 'ABC-001', 'Test Movie', 'waiting', 2);
	`)
	if err != nil {
		t.Fatal(err)
	}

	if err := migrateSubscriptions(t.Context(), store.db); err != nil {
		t.Fatal(err)
	}

	subs, err := store.Client.Subscription.Query().All(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}
	sub := subs[0]
	if sub.ID != 1 || sub.TargetID != "m-001" || sub.Code != "ABC-001" || sub.Kind != "movie" || sub.Status != "waiting" || sub.Checks != 2 {
		t.Fatalf("unexpected subscription content: %+v", sub)
	}

	// Verify monitors table was dropped
	var count int
	err = store.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='monitors'").Scan(&count)
	if err != nil || count != 0 {
		t.Fatalf("monitors table should be dropped, count=%d, err=%v", count, err)
	}
}
