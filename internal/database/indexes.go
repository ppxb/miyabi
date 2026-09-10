package database

import (
	"context"
	"database/sql"
	"fmt"
)

// Ent cannot describe expression indexes. Keep the existing JSON storage and
// index the stable scope/hash fields used by both offline history queries.
// Other task types do not pay for these indexes or carry their larger documents.
func createTaskHistoryIndexes(ctx context.Context, database *sql.DB) error {
	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS task_offline_source_history ON tasks (
			json_extract(payload, '$.account_id'), json_extract(payload, '$.directory_id'),
			json_type(payload, '$.hash'), json_extract(payload, '$.hash'), id DESC
		) WHERE type = 'offline'`,
		`CREATE INDEX IF NOT EXISTS task_offline_movie_history ON tasks (
			json_extract(payload, '$.account_id'), json_extract(payload, '$.javdb_id'),
			json_type(payload, '$.hash'), json_extract(payload, '$.hash'), id DESC
		) WHERE type = 'offline'`,
	} {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create offline task history index: %w", err)
		}
	}
	return nil
}
