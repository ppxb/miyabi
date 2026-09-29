package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const viewedMoviesLegacySetting = "browse.viewed_movies"

func migrateViewedMovies(ctx context.Context, tx *sql.Tx) error {
	var raw []byte
	err := tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", viewedMoviesLegacySetting).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var payload struct {
		IDs []string `json:"ids"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("decode legacy viewed movies: %w", err)
	}
	now := time.Now().UTC()
	statement, err := tx.PrepareContext(ctx, "INSERT INTO viewed_movies (javdb_id, viewed_at) VALUES (?, ?) ON CONFLICT(javdb_id) DO NOTHING")
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, id := range payload.IDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, err := statement.ExecContext(ctx, id, now); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, "DELETE FROM settings WHERE key = ?", viewedMoviesLegacySetting)
	return err
}
