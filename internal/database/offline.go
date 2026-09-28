package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// migrateOfflineDownloads preserves IDs referenced by subscriptions and scan payloads.
// Copy and removal share a transaction, so a failed migration leaves all old rows intact.
func migrateOfflineDownloads(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id, created_at, updated_at, status, progress, error, payload FROM tasks WHERE type = 'offline' ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, progress int
		var createdAt, updatedAt any
		var status string
		var message sql.NullString
		var raw []byte
		if err := rows.Scan(&id, &createdAt, &updatedAt, &status, &progress, &message, &raw); err != nil {
			return err
		}
		var payload struct {
			Code             string   `json:"code"`
			JavDBID          string   `json:"javdb_id"`
			Hash             string   `json:"hash"`
			InfoHash         string   `json:"info_hash"`
			AccountID        string   `json:"account_id"`
			DirectoryID      string   `json:"directory_id"`
			FileID           string   `json:"file_id"`
			FileIDs          []string `json:"file_ids"`
			ScanTaskID       int      `json:"scan_task_id"`
			AwaitingLocation bool     `json:"awaiting_location"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return fmt.Errorf("decode offline task %d: %w", id, err)
		}
		if payload.Hash == "" {
			return fmt.Errorf("offline task %d is missing magnet hash", id)
		}
		if status == "queued" {
			status = "running"
		}
		if payload.FileIDs == nil {
			payload.FileIDs = []string{}
		}
		fileIDs, err := json.Marshal(payload.FileIDs)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO offline_downloads
   (id, created_at, updated_at, status, progress, error, code, javdb_id, hash, info_hash,
    account_id, directory_id, file_id, file_ids, scan_task_id, awaiting_location)
   VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, createdAt, updatedAt, status, progress, message, payload.Code, payload.JavDBID,
			payload.Hash, payload.InfoHash, payload.AccountID, payload.DirectoryID, payload.FileID,
			string(fileIDs), payload.ScanTaskID, payload.AwaitingLocation)
		if err != nil {
			return fmt.Errorf("migrate offline task %d: %w", id, err)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, statement := range []string{
		`DELETE FROM tasks WHERE type = 'offline'`,
		`DROP INDEX IF EXISTS task_offline_source_history`,
		`DROP INDEX IF EXISTS task_offline_movie_history`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}
