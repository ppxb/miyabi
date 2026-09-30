package database

import (
	"context"
	"database/sql"
	"fmt"
)

// These legacy columns must be removed before Ent can update the schema.
const preSchemaVersion = 1

// Versions are one-based positions. Append new data migrations; never reorder
// existing entries. Ent manages tables and current field indexes. Versioned
// steps also retire obsolete indexes and create SQLite expression indexes.
var migrations = []struct {
	name string
	run  func(context.Context, *sql.Tx) error
}{
	{"remove player storage", dropPlayerStorage},
	{"migrate subscriptions", migrateSubscriptions},
	{"migrate offline downloads", migrateOfflineDownloads},
	{"migrate metadata snapshots", migrateMetadataSnapshots},
	{"migrate viewed movies", migrateViewedMovies},
	{"remove 115 subtitle storage", dropPanSubtitleStorage},
	{"index metadata workflow tasks", indexMetadataWorkflowTasks},
	{"remove obsolete task queue index", dropTaskCreatedAtIndex},
	{"backfill movie matching keys", backfillMovieMatchKeys},
	{"index cover task parents", indexCoverTaskParents},
}

func runMigrations(ctx context.Context, db *sql.DB, through int) error {
	for {
		applied, err := migrateNext(ctx, db, through)
		if err != nil || !applied {
			return err
		}
	}
}

func migrateNext(ctx context.Context, db *sql.DB, through int) (bool, error) {
	// The DSN's immediate transaction reserves the writer before reading the
	// version, so concurrent openers cannot execute the same step twice.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return false, err
	}
	if version < 0 || version > len(migrations) {
		return false, fmt.Errorf("unsupported database migration version %d (latest %d)", version, len(migrations))
	}
	if version >= through {
		return false, nil
	}
	step := migrations[version]
	if err := step.run(ctx, tx); err != nil {
		return false, fmt.Errorf("migration %d (%s): %w", version+1, step.name, err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version+1)); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
