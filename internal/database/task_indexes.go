package database

import (
	"context"
	"database/sql"
)

// Ent manages field indexes; the workflow index also uses a JSON expression.
func createTaskIndexes(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS task_scan_workflow
		ON tasks (json_extract(payload, '$.scan_task_id'), type, status, updated_at DESC, id DESC)`)
	return err
}
