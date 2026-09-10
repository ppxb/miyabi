package database

import (
	"fmt"
	"testing"
	"time"
)

// Synthetic catalogue data keeps performance checks independent of user data.
func BenchmarkLibraryQueries(b *testing.B) {
	store, err := Open(b.Context(), b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	tx, err := store.db.BeginTx(b.Context(), nil)
	if err != nil {
		b.Fatal(err)
	}
	defer tx.Rollback()
	movies, err := tx.Prepare("INSERT INTO movies (created_at, updated_at, code, title, scrape_status, fanarts) VALUES (?, ?, ?, '', 'done', '[]')")
	if err != nil {
		b.Fatal(err)
	}
	defer movies.Close()
	files, err := tx.Prepare("INSERT INTO files (created_at, updated_at, file_id, name, size, account_id, root_id, movie_files) VALUES (?, ?, ?, 'video.mp4', 1024, '100', '10', ?)")
	if err != nil {
		b.Fatal(err)
	}
	defer files.Close()
	tasks, err := tx.Prepare("INSERT INTO tasks (created_at, updated_at, type, status, payload, progress) VALUES (?, ?, ?, 'done', '{}', 100)")
	if err != nil {
		b.Fatal(err)
	}
	defer tasks.Close()
	now := time.Now()
	for i := range 5000 {
		if _, err := movies.Exec(now, now, fmt.Sprintf("ABP-%05d", i+1)); err != nil {
			b.Fatal(err)
		}
	}
	for i := range 20000 {
		if _, err := files.Exec(now, now, fmt.Sprint(i+1), i%5000+1); err != nil {
			b.Fatal(err)
		}
	}
	for i := range 50000 {
		kind := "cover"
		if i%1000 == 0 {
			kind = "scan"
		}
		if _, err := tasks.Exec(now, now, kind); err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	b.Run("MovieFiles", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var count int
			if err := store.db.QueryRowContext(b.Context(), "SELECT COUNT(*) FROM files WHERE movie_files=? AND account_id=? AND root_id=?", 2500, "100", "10").Scan(&count); err != nil || count != 4 {
				b.Fatalf("file count=%d err=%v", count, err)
			}
		}
	})
	b.Run("RecentScans", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			rows, err := store.db.QueryContext(b.Context(), "SELECT id, payload, status FROM tasks WHERE type='scan' ORDER BY id DESC LIMIT 20")
			if err != nil {
				b.Fatal(err)
			}
			count := 0
			for rows.Next() {
				var id int
				var payload, status string
				if err := rows.Scan(&id, &payload, &status); err != nil {
					rows.Close()
					b.Fatal(err)
				}
				count++
			}
			err = rows.Err()
			rows.Close()
			if err != nil || count != 20 {
				b.Fatalf("scan count=%d err=%v", count, err)
			}
		}
	})
}
