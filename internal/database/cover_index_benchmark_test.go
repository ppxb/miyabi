package database

import (
	"database/sql"
	"fmt"
	"testing"
	"time"
)

// Measure the parent lookup against a synthetic history, including absent parents.
func BenchmarkCoverParentLookup(b *testing.B) {
	store, err := Open(b.Context(), b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	tx, err := store.db.BeginTx(b.Context(), nil)
	if err != nil {
		b.Fatal(err)
	}
	defer tx.Rollback()
	insert, err := tx.PrepareContext(b.Context(), `INSERT INTO tasks
		(created_at, updated_at, type, status, payload, progress) VALUES (?, ?, ?, 'done', ?, 100)`)
	if err != nil {
		b.Fatal(err)
	}
	defer insert.Close()
	now := time.Now()
	for i := 1; i <= 20000; i++ {
		kind := "cover"
		if i%4 == 1 {
			kind = "scrape"
		}
		payload := fmt.Sprintf(`{"scan_task_id":1,"scrape_task_id":%d,"code":"TEST-001"}`, i)
		if _, err := insert.ExecContext(b.Context(), now, now, kind, payload); err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	for _, parent := range []int{20000, 20001} {
		b.Run(fmt.Sprint(parent), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var id int
				err := store.db.QueryRowContext(b.Context(),
					"SELECT id FROM tasks WHERE type=? AND json_extract(payload, '$.scrape_task_id')=? LIMIT 1", "cover", parent).Scan(&id)
				if parent == 20000 && (err != nil || id != 20000) || parent == 20001 && err != sql.ErrNoRows {
					b.Fatalf("parent=%d id=%d err=%v", parent, id, err)
				}
			}
		})
	}
}
