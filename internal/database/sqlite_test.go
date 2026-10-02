package database

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/nfo"
)

func TestLibraryIndexesAndRecordsSurviveReopen(t *testing.T) {
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
	doc := &nfo.Movie{Code: "ABP-001", Title: "Preserved title", Zone: domain.ZoneUnknown,
		IDs:          []nfo.UniqueID{{Type: "javdb", Value: "movie-id"}},
		FieldSources: map[string]string{"title": "javdb"},
		Images:       []domain.ImageCandidate{{Provider: "javdb", Role: "preview", URL: "https://example.com/preview.jpg"}},
	}
	movie := store.Client.Movie.Create().SetCode(doc.Code).SetTitle(doc.Title).SetMetadata(doc).SaveX(t.Context())
	file := store.Client.File.Create().SetFileID("video").SetName("ABP-001.mp4").SetSize(1024).
		SetAccountID("100").SetRootID("10").SetMovie(movie).SaveX(t.Context())
	job := store.Client.Task.Create().SetType("scan").SetPayload(json.RawMessage(`{"fixture":"preserved"}`)).SaveX(t.Context())
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Client.Movie.GetX(t.Context(), movie.ID); got.Title != movie.Title || !reflect.DeepEqual(got.Metadata, doc) {
		t.Fatal("reopen changed movie metadata")
	}
	if got := store.Client.File.GetX(t.Context(), file.ID); got.MovieID == nil || *got.MovieID != movie.ID || got.FileID != file.FileID {
		t.Fatal("reopen changed a file association")
	}
	if got := store.Client.Task.GetX(t.Context(), job.ID); string(got.Payload) != `{"fixture":"preserved"}` {
		t.Fatal("reopen changed a stored task")
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
	// Preserve numeric IDs and the stored checkpoint exactly across restarts.
	payload := `{"source":{"account_id":"100","directory":{"id":"10","path":"/Movies"}},"scan":{"stage":"scanning","files_scanned":7},"offline_task_id":9007199254740993,"scan_id":"current-scan","checkpoint":"[]"}`
	if _, err := store.db.ExecContext(t.Context(), "UPDATE tasks SET payload = ? WHERE id = ?", payload, job.ID); err != nil {
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
	if string(loaded.Payload) != payload {
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
