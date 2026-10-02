package database

import (
	"strings"
	"testing"
)

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
	job := store.Client.OfflineDownload.Create().SetHash("fixture").SetAccountID("100").SetDirectoryID("10").SetJavdbID("movie").SaveX(t.Context())
	for range 2 {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		store, err = Open(t.Context(), directory)
		if err != nil {
			t.Fatal(err)
		}
		if got := store.Client.OfflineDownload.GetX(t.Context(), job.ID); got.Hash != job.Hash || got.Status != job.Status {
			t.Fatal("reopen changed a download")
		}
		for _, query := range []struct{ field, value, index string }{
			{"status", "running", "offlinedownload_account_id_status_hash"},
			{"directory_id", "10", "offlinedownload_account_id_directory_id_hash_id"},
			{"javdb_id", "movie", "offlinedownload_account_id_javdb_id_hash_id"},
		} {
			rows, err := store.db.QueryContext(t.Context(),
				"EXPLAIN QUERY PLAN SELECT MAX(id) FROM offline_downloads WHERE account_id = ? AND "+query.field+" = ? GROUP BY hash",
				"100", query.value)
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
