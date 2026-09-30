package database

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
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
	// Recreate missing schema-managed indexes without changing downloads.
	for _, name := range []string{"offlinedownload_account_id_directory_id_hash_id", "offlinedownload_account_id_javdb_id_hash_id"} {
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
		if got := store.Client.OfflineDownload.GetX(t.Context(), job.ID); got.Hash != job.Hash || got.Status != job.Status {
			t.Fatal("history index migration changed a download")
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

func TestMigrateOfflineDownloadsPreservesHistoryAndReferences(t *testing.T) {
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
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	var legacy []*ent.Task
	for i, status := range []task.Status{task.StatusQueued, task.StatusRunning, task.StatusDone, task.StatusFailed} {
		payload := json.RawMessage(fmt.Sprintf(`{"code":"ABP-001","javdb_id":"movie","hash":"hash-%d","info_hash":"remote-%d","account_id":"100","directory_id":"10","file_id":"folder","file_ids":["video-1","video-2"],"scan_task_id":99,"awaiting_location":true}`, i, i))
		create := store.Client.Task.Create().SetType("offline").SetStatus(status).SetProgress(i * 25).
			SetCreatedAt(createdAt).SetUpdatedAt(updatedAt).SetPayload(payload)
		if status == task.StatusFailed {
			create.SetError("preserved failure")
		}
		legacy = append(legacy, create.SaveX(ctx))
	}
	scanPayload := json.RawMessage(fmt.Sprintf(`{"offline_task_id":%d,"target_id":"folder"}`, legacy[2].ID))
	scan := store.Client.Task.Create().SetType("scan").SetPayload(scanPayload).SaveX(ctx)
	sub := store.Client.Subscription.Create().SetTargetID("movie").SetTaskID(legacy[2].ID).SaveX(ctx)
	// Model an existing installation without the new table and with its old JSON indexes.
	for _, statement := range []string{
		`PRAGMA user_version = 0`,
		`DROP TABLE offline_downloads`,
		`CREATE INDEX task_offline_source_history ON tasks (json_extract(payload, '$.account_id'), json_extract(payload, '$.directory_id'), json_type(payload, '$.hash'), json_extract(payload, '$.hash'), id DESC) WHERE type = 'offline'`,
		`CREATE INDEX task_offline_movie_history ON tasks (json_extract(payload, '$.account_id'), json_extract(payload, '$.javdb_id'), json_type(payload, '$.hash'), json_extract(payload, '$.hash'), id DESC) WHERE type = 'offline'`,
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		store, err = Open(ctx, directory)
		if err != nil {
			t.Fatal(err)
		}
		if count := store.Client.OfflineDownload.Query().CountX(ctx); count != len(legacy) {
			t.Fatalf("migrated %d rows, want %d", count, len(legacy))
		}
		for i, old := range legacy {
			got := store.Client.OfflineDownload.GetX(ctx, old.ID)
			wantStatus := string(old.Status)
			if old.Status == task.StatusQueued {
				wantStatus = "running"
			}
			if string(got.Status) != wantStatus || got.Progress != old.Progress ||
				!got.CreatedAt.Equal(old.CreatedAt) || !got.UpdatedAt.Equal(old.UpdatedAt) ||
				got.Code != "ABP-001" || got.JavdbID != "movie" || got.Hash != fmt.Sprintf("hash-%d", i) ||
				got.InfoHash != fmt.Sprintf("remote-%d", i) || got.AccountID != "100" || got.DirectoryID != "10" ||
				got.FileID != "folder" || !slices.Equal(got.FileIds, []string{"video-1", "video-2"}) ||
				got.ScanTaskID != 99 || !got.AwaitingLocation {
				t.Fatalf("download changed during migration: %+v", got)
			}
			if old.Error == nil && got.Error != nil || old.Error != nil && (got.Error == nil || *old.Error != *got.Error) {
				t.Fatalf("download error changed: %+v", got)
			}
		}
		if count := store.Client.Task.Query().Where(task.TypeEQ("offline")).CountX(ctx); count != 0 {
			t.Fatalf("%d legacy downloads remain in the task queue", count)
		}
		if got := store.Client.Task.GetX(ctx, scan.ID); string(got.Payload) != string(scanPayload) {
			t.Fatalf("scan reference changed: %s", got.Payload)
		}
		if got := store.Client.Subscription.GetX(ctx, sub.ID); got.TaskID == nil || *got.TaskID != legacy[2].ID {
			t.Fatalf("subscription reference changed: %+v", got)
		}
		var indexes int
		if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name IN ('task_offline_source_history', 'task_offline_movie_history')`).Scan(&indexes); err != nil || indexes != 0 {
			t.Fatalf("legacy indexes remain: %d, %v", indexes, err)
		}
	}
	next := store.Client.OfflineDownload.Create().SetHash("new-download").SaveX(ctx)
	if next.ID <= legacy[len(legacy)-1].ID {
		t.Fatalf("download ID sequence was not preserved: %d", next.ID)
	}
}

func TestMigrateOfflineDownloadsRollsBackInvalidHistory(t *testing.T) {
	for _, raw := range []string{`{`, `null`, `{}`, `{"hash":123}`, `{"hash":"bad-files","file_ids":[123]}`} {
		t.Run(raw, func(t *testing.T) {
			ctx := t.Context()
			store, err := Open(ctx, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			setMigrationVersion(t, store, 2)
			// This legacy database predates both task JSON expression indexes.
			for _, name := range []string{"task_scan_workflow", "task_cover_parent"} {
				if _, err := store.db.ExecContext(ctx, "DROP INDEX "+name); err != nil {
					t.Fatal(err)
				}
			}
			good := store.Client.Task.Create().SetType("offline").SetPayload(json.RawMessage(`{"hash":"valid"}`)).SaveX(ctx)
			bad := store.Client.Task.Create().SetType("offline").SaveX(ctx)
			if _, err := store.db.ExecContext(ctx, `UPDATE tasks SET payload = ? WHERE id = ?`, raw, bad.ID); err != nil {
				t.Fatal(err)
			}
			if err := runMigrations(ctx, store.db, len(migrations)); err == nil || !strings.Contains(err.Error(), fmt.Sprint(bad.ID)) {
				t.Fatalf("invalid history did not fail with its row ID: %v", err)
			}
			if got := migrationVersion(t, store.db); got != 2 {
				t.Fatalf("failed offline migration advanced version to %d", got)
			}
			if count := store.Client.OfflineDownload.Query().CountX(ctx); count != 0 {
				t.Fatalf("%d copies escaped rollback", count)
			}
			if count := store.Client.Task.Query().Where(task.TypeEQ("offline")).CountX(ctx); count != 2 {
				t.Fatalf("legacy history lost after rollback: %d", count)
			}
			if got := store.Client.Task.GetX(ctx, good.ID); string(got.Payload) != string(good.Payload) {
				t.Fatal("valid source row changed")
			}
			var saved string
			if err := store.db.QueryRowContext(ctx, `SELECT payload FROM tasks WHERE id = ?`, bad.ID).Scan(&saved); err != nil || saved != raw {
				t.Fatalf("invalid source row changed: %q, %v", saved, err)
			}
			// Correcting the offending row must allow a retry with no partial copies or ID conflicts.
			if _, err := store.db.ExecContext(ctx, `UPDATE tasks SET payload = ? WHERE id = ?`, `{"hash":"corrected"}`, bad.ID); err != nil {
				t.Fatal(err)
			}
			if err := runMigrations(ctx, store.db, len(migrations)); err != nil {
				t.Fatal(err)
			}
			if count := store.Client.OfflineDownload.Query().CountX(ctx); count != 2 {
				t.Fatalf("retry copied %d rows", count)
			}
			if got := store.Client.OfflineDownload.GetX(ctx, good.ID); len(got.FileIds) != 0 || got.ScanTaskID != 0 || got.AwaitingLocation {
				t.Fatalf("absent optional fields changed: %+v", got)
			}
		})
	}
}
