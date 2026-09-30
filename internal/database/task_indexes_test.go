package database

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/ent/task"
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

func TestTaskQueueIndexUpgradePreservesHistoryAndRecords(t *testing.T) {
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
	var ids []int
	for _, entry := range []struct {
		kind   string
		status task.Status
	}{
		{"cover", task.StatusQueued},
		{"scan", task.StatusRunning},
		{"scan", task.StatusDone},
		{"scrape", task.StatusQueued},
		{"subscription_batch", task.StatusFailed},
		{"scan", task.StatusQueued},
		{"subscription_batch", task.StatusQueued},
		{"cover", task.StatusRunning},
	} {
		job := store.Client.Task.Create().SetType(entry.kind).SetStatus(entry.status).
			SetPayload(json.RawMessage(`{"scan_task_id":12,"future":{"keep":true}}`)).
			SetProgress(42).SetError("preserved").SaveX(ctx)
		ids = append(ids, job.ID)
	}
	before := store.Client.Task.Query().Order(task.ByID()).AllX(ctx)
	queries := []struct {
		name, sql, index string
		args             []any
		want             []int
		needsSort        bool
	}{
		{"batch claim", "SELECT id, payload FROM tasks WHERE type IN (?) AND status=? ORDER BY id LIMIT 1", "task_type_status", []any{"subscription_batch", "queued"}, []int{ids[6]}, false},
		{"library claim", "SELECT id, payload FROM tasks WHERE type IN (?, ?, ?) AND status=? ORDER BY id LIMIT 1", "task_type_status", []any{"scan", "scrape", "cover", "queued"}, []int{ids[0]}, true},
		{"scan history", "SELECT id, payload FROM tasks WHERE type=? ORDER BY id DESC LIMIT 20", "task_type", []any{"scan"}, []int{ids[5], ids[2], ids[1]}, false},
		{"batch history", "SELECT id, payload FROM tasks WHERE type=? ORDER BY id DESC LIMIT 5", "task_type", []any{"subscription_batch"}, []int{ids[6], ids[4]}, false},
	}
	checkQueries := func() {
		t.Helper()
		for _, query := range queries {
			plan := taskQueryPlan(t, store, query.sql, query.args...)
			if !strings.Contains(plan, "INDEX "+query.index+" (") || strings.Contains(plan, "TEMP B-TREE") != query.needsSort {
				t.Fatalf("%s query plan: %s", query.name, plan)
			}
			if query.index == "task_type_status" && !strings.Contains(plan, "type=? AND status=?") {
				t.Fatalf("claim did not constrain both index columns: %s", plan)
			}
			rows, err := store.db.QueryContext(ctx, query.sql, query.args...)
			if err != nil {
				t.Fatal(err)
			}
			var got []int
			for rows.Next() {
				var id int
				var payload []byte
				if err := rows.Scan(&id, &payload); err != nil {
					rows.Close()
					t.Fatal(err)
				}
				got = append(got, id)
			}
			err = rows.Err()
			rows.Close()
			if err != nil || !slices.Equal(got, query.want) {
				t.Fatalf("%s results: %v, %v", query.name, got, err)
			}
			t.Logf("%s: %s", query.name, plan)
		}
		recovery := taskQueryPlan(t, store, "UPDATE tasks SET status='queued', progress=0, error=NULL WHERE type IN (?, ?, ?) AND status=?", "scan", "scrape", "cover", "running")
		if !strings.Contains(recovery, "INDEX task_type_status (type=? AND status=?)") {
			t.Fatalf("recovery plan: %s", recovery)
		}
		active := taskQueryPlan(t, store, "SELECT id, payload FROM tasks WHERE type=? AND status IN (?, ?)", "subscription_batch", "queued", "running")
		if !strings.Contains(active, "INDEX task_type_status (type=? AND status=?)") {
			t.Fatalf("active batch plan: %s", active)
		}
	}
	checkQueries() // New installations get the indexes from the Ent schema.
	for _, statement := range []string{
		"DROP INDEX task_type_status",
		"CREATE INDEX task_status_created_at ON tasks (status, created_at)",
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	setMigrationVersion(t, store, 7)
	for range 2 {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		store, err = Open(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := migrationVersion(t, store.db); got != len(migrations) {
			t.Fatalf("migration version=%d", got)
		}
		var obsolete int
		if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='task_status_created_at'").Scan(&obsolete); err != nil || obsolete != 0 {
			t.Fatalf("obsolete index survived reopen: %d, %v", obsolete, err)
		}
		after := store.Client.Task.Query().Order(task.ByID()).AllX(ctx)
		if len(after) != len(before) {
			t.Fatal("index upgrade changed the task count")
		}
		for i, got := range after {
			want := before[i]
			if got.ID != want.ID || got.Type != want.Type || got.Status != want.Status || got.Progress != want.Progress ||
				string(got.Payload) != string(want.Payload) || got.Error == nil || *got.Error != *want.Error ||
				!got.CreatedAt.Equal(want.CreatedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) {
				t.Fatalf("index upgrade changed task %d", want.ID)
			}
		}
		checkQueries()
	}
}

func taskQueryPlan(t *testing.T, store *Store, query string, args ...any) string {
	t.Helper()
	rows, err := store.db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(details, "; ")
}
