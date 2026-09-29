package database

import (
	"context"
	"database/sql"
	"fmt"
)

func migrateSubscriptions(ctx context.Context, tx *sql.Tx) error {
	var count int
	err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='monitors'").Scan(&count)
	if err != nil || count == 0 {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO subscriptions (
			id, created_at, updated_at, kind, target_id, code, title, cover, release_date,
			status, hash, task_id, next_check_at, last_checked_at, checks, error, auto_download, zone, cursor
		)
		SELECT
			id, created_at, updated_at, 'movie', movie_id, code, title, cover, release_date,
			status, hash, task_id, next_check_at, last_checked_at, checks, error, 1, '', ''
		FROM monitors;
		DROP TABLE monitors;
	`)
	return err
}

// dropPlayerStorage removes the watch history and player-only columns left by
// the in-app player. Legacy NOT NULL columns would otherwise reject new rows.
func dropPlayerStorage(ctx context.Context, tx *sql.Tx) error {
	statements := []string{
		"DROP TABLE IF EXISTS watch_histories",
		"DROP INDEX IF EXISTS subtitle_movie_id_is_default",
	}
	for _, column := range []struct{ table, name string }{
		{"movies", "watched"},
		{"subtitles", "display_name"},
		{"subtitles", "offset_ms"},
		{"subtitles", "is_default"},
	} {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?",
			column.table, column.name).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			statements = append(statements, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", column.table, column.name))
		}
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
