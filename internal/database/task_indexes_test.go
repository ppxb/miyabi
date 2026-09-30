package database

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTaskWorkflowIndexUpgradesAndSurvivesReopen(t *testing.T) {
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
	payload := json.RawMessage(`{"scan_task_id":12,"scrape_task_id":34,"future":{"keep":true}}`)
	job := store.Client.Task.Create().SetType("cover").SetPayload(payload).SetProgress(40).SetError("preserved").SaveX(ctx)
	if _, err := store.db.ExecContext(ctx, "DROP INDEX task_scan_workflow"); err != nil {
		t.Fatal(err)
	}
	setMigrationVersion(t, store, 6)
	for range 2 {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		store, err = Open(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		got := store.Client.Task.GetX(ctx, job.ID)
		if string(got.Payload) != string(payload) || got.Status != job.Status || got.Progress != job.Progress ||
			got.Error == nil || *got.Error != *job.Error || !got.UpdatedAt.Equal(job.UpdatedAt) {
			t.Fatalf("index migration changed task: %+v", got)
		}
		// Match metadataGroups' partition, ordering and parameterized filter.
		rows, err := store.db.QueryContext(ctx, `EXPLAIN QUERY PLAN SELECT id,
			COUNT(*) OVER (PARTITION BY json_extract(payload, '$.scan_task_id'), type, status),
			ROW_NUMBER() OVER (PARTITION BY json_extract(payload, '$.scan_task_id'), type, status ORDER BY updated_at DESC, id DESC)
			FROM tasks WHERE type IN (?, ?) AND json_extract(payload, '$.scan_task_id') IN (?, ?)`, "scrape", "cover", 12, 13)
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
		if err != nil || !strings.Contains(plan, "USING INDEX task_scan_workflow") || strings.Contains(plan, "TEMP B-TREE") {
			t.Fatalf("workflow index not used: %s, %v", plan, err)
		}
	}
	// Updating the JSON association must also update the expression index.
	store.Client.Task.UpdateOneID(job.ID).SetPayload(json.RawMessage(`{"scan_task_id":13}`)).ExecX(ctx)
	var old, current int
	if err := store.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM tasks WHERE json_extract(payload, '$.scan_task_id') = 12),
		(SELECT COUNT(*) FROM tasks WHERE json_extract(payload, '$.scan_task_id') = 13)`).Scan(&old, &current); err != nil || old != 0 || current != 1 {
		t.Fatalf("association index not updated: old=%d current=%d err=%v", old, current, err)
	}
}
