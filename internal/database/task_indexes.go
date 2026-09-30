package database

import (
	"context"
	"database/sql"
)

func indexMetadataWorkflowTasks(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS task_scan_workflow
		ON tasks (json_extract(payload, '$.scan_task_id'), type, status, updated_at DESC, id DESC)`)
	return err
}

// Ent creates the replacement (type, status) index before data migrations run.
func dropTaskCreatedAtIndex(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, "DROP INDEX IF EXISTS task_status_created_at")
	return err
}

// Any committed cover task proves its metadata transaction succeeded, regardless
// of status. Keep the JSON value's type and allow existing duplicate parents.
func indexCoverTaskParents(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS task_cover_parent
		ON tasks (json_extract(payload, '$.scrape_task_id'), type)`)
	return err
}
