package database

import (
	"context"
	"database/sql"
	"fmt"
)

// dropPanSubtitleStorage removes retired 115 metadata, never exported files.
func dropPanSubtitleStorage(ctx context.Context, tx *sql.Tx) error {
	var columns []string
	for _, name := range []string{"file_id", "pick_code"} {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM pragma_table_info('subtitles') WHERE name = ?", name).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			columns = append(columns, name)
		}
	}
	predicate := "source = '115'"
	for _, name := range columns {
		predicate += " OR " + name + " != ''"
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM subtitles WHERE "+predicate); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DROP INDEX IF EXISTS subtitle_file_id"); err != nil {
		return err
	}
	for _, name := range columns {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE subtitles DROP COLUMN %s", name)); err != nil {
			return err
		}
	}
	return nil
}
