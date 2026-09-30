package database

import (
	"context"
	"database/sql"

	"github.com/ppxb/miyabi/internal/codeid"
)

func backfillMovieMatchKeys(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, "SELECT id, code FROM movies ORDER BY id")
	if err != nil {
		return err
	}
	defer rows.Close()
	update, err := tx.PrepareContext(ctx, "UPDATE movies SET canonical_code = ? WHERE id = ?")
	if err != nil {
		return err
	}
	defer update.Close()
	for rows.Next() {
		var id int
		var code string
		if err := rows.Scan(&id, &code); err != nil {
			return err
		}
		// Only the derived key changes; metadata, associations and timestamps stay intact.
		if _, err := update.ExecContext(ctx, codeid.MatchKey(code), id); err != nil {
			return err
		}
	}
	return rows.Err()
}
