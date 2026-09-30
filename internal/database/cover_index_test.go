package database

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/ent/task"
)

func TestCoverParentIndexUpgradesWithoutChangingTasks(t *testing.T) {
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
	for _, status := range []task.Status{task.StatusQueued, task.StatusRunning, task.StatusDone, task.StatusFailed} {
		store.Client.Task.Create().SetType("cover").SetStatus(status).SetProgress(40).SetError("preserved").
			SetPayload(json.RawMessage(`{"scrape_task_id":34,"scan_task_id":12,"future":{"id":9007199254740993}}`)).ExecX(ctx)
	}
	for _, entry := range []struct{ kind, payload string }{
		{"scrape", `{"scrape_task_id":90}`}, {"cover", `{"scrape_task_id":"90"}`},
		{"cover", `{}`}, {"cover", `{"scrape_task_id":null}`},
	} {
		store.Client.Task.Create().SetType(entry.kind).SetPayload(json.RawMessage(entry.payload)).ExecX(ctx)
	}
	before := store.Client.Task.Query().Order(task.ByID()).AllX(ctx)
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	// Use Ent's JSON predicate generator, as Scrape's Exist query does.
	builder := entsql.Dialect(dialect.SQLite)
	table := builder.Table(task.Table)
	query, args := builder.Select(table.C(task.FieldID)).From(table).Where(entsql.And(
		entsql.EQ(table.C(task.FieldType), "cover"),
		sqljson.ValueEQ(table.C(task.FieldPayload), 34, sqljson.Path("scrape_task_id")),
	)).Limit(1).Query()
	checkPlan := func() {
		t.Helper()
		plan := queryPlan(t, store, query, args...)
		if !strings.Contains(plan, "task_cover_parent (<expr>=? AND type=?)") {
			t.Fatalf("parent and type not constrained by index: %s", plan)
		}
		t.Log(plan)
	}
	checkPlan() // Fresh databases also get the expression index.
	if _, err := store.db.ExecContext(ctx, "DROP INDEX task_cover_parent"); err != nil {
		t.Fatal(err)
	}
	t.Logf("before upgrade: %s", queryPlan(t, store, query, args...))
	setMigrationVersion(t, store, 9)
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
		after := store.Client.Task.Query().Order(task.ByID()).AllX(ctx)
		afterJSON, err := json.Marshal(after)
		if err != nil || !bytes.Equal(beforeJSON, afterJSON) {
			t.Fatalf("index upgrade changed task records: %v", err)
		}
		for i, record := range after {
			if !bytes.Equal(record.Payload, before[i].Payload) {
				t.Fatal("index upgrade rewrote a payload")
			}
		}
		checkPlan()
	}
	var id int
	args[1] = 90
	if err := store.db.QueryRowContext(ctx, query, args...).Scan(&id); err != sql.ErrNoRows {
		t.Fatalf("wrong task type or string parent matched integer parent: id=%d err=%v", id, err)
	}
	store.Client.Task.UpdateOneID(before[0].ID).SetPayload(json.RawMessage(`{"scrape_task_id":90}`)).ExecX(ctx)
	if err := store.db.QueryRowContext(ctx, query, args...).Scan(&id); err != nil || id != before[0].ID {
		t.Fatalf("updated parent missing from index: id=%d err=%v", id, err)
	}
	store.Client.Task.UpdateOneID(before[0].ID).SetType("scrape").ExecX(ctx)
	if err := store.db.QueryRowContext(ctx, query, args...).Scan(&id); err != sql.ErrNoRows {
		t.Fatalf("changed task type left an indexed cover match: %v", err)
	}
}
