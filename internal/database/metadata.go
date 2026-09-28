package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ppxb/miyabi/internal/domain"
)

const metadataSnapshotMigration = "migration.movie_metadata_snapshot"

// migrateMetadataSnapshots moves export state out of task history once. A cover
// snapshot also marked a committed export awaiting queue completion after a crash.
func migrateMetadataSnapshots(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var migrated bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM settings WHERE key = ?)`, metadataSnapshotMigration).Scan(&migrated); err != nil || migrated {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, payload FROM tasks WHERE type = 'cover'
		AND (status = 'done' OR json_type(payload, '$.snapshot') = 'object') ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return err
		}
		var input struct {
			MovieID  int                      `json:"movie_id"`
			Source   domain.LibrarySource     `json:"source"`
			Snapshot *domain.MetadataSnapshot `json:"snapshot"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return fmt.Errorf("decode cover task %d: %w", id, err)
		}
		var snapshot any
		if input.Snapshot != nil {
			input.Snapshot.AccountID = input.Source.AccountID
			input.Snapshot.DirectoryID = input.Source.Directory.ID
			encoded, err := json.Marshal(input.Snapshot)
			if err != nil {
				return err
			}
			snapshot = string(encoded)
		}
		// Keep the newest export for a source still represented in the index.
		// A newer legacy job without a snapshot invalidates that source's cache.
		if _, err := tx.ExecContext(ctx, `UPDATE movies SET metadata_snapshot = ? WHERE id = ?
			AND EXISTS(SELECT 1 FROM files WHERE movie_files = movies.id AND account_id = ? AND root_id = ?)
			AND (? IS NOT NULL OR (json_extract(metadata_snapshot, '$.account_id') = ?
				AND json_extract(metadata_snapshot, '$.directory_id') = ?))`,
			snapshot, input.MovieID, input.Source.AccountID, input.Source.Directory.ID,
			snapshot, input.Source.AccountID, input.Source.Directory.ID); err != nil {
			return fmt.Errorf("migrate cover task %d: %w", id, err)
		}
		if input.Snapshot != nil {
			// JSON functions retain unrelated fields and integer IDs exactly.
			if _, err := tx.ExecContext(ctx, `UPDATE tasks SET payload =
				json_remove(json_set(payload, '$.completed', json('true')), '$.snapshot') WHERE id = ?`, id); err != nil {
				return err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value, created_at, updated_at)
		VALUES (?, 'true', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, metadataSnapshotMigration); err != nil {
		return err
	}
	return tx.Commit()
}
