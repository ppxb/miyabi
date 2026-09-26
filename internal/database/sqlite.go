package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/ppxb/miyabi/internal/ent"
	_ "modernc.org/sqlite"
)

type Store struct {
	Client *ent.Client
	db     *sql.DB
}

func Open(ctx context.Context, dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	databasePath, err := filepath.Abs(filepath.Join(dataDir, "miyabi.db"))
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}

	db, err := sql.Open("sqlite", sqliteDSN(databasePath))
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}

	driver := entsql.OpenDB(dialect.SQLite, db)
	opts := []ent.Option{
		ent.Driver(driver),
		ent.Log(func(args ...any) {
			slog.Debug(fmt.Sprint(args...))
		}),
	}
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		opts = append(opts, ent.Debug())
	}
	client := ent.NewClient(opts...)
	if err := dropPlayerStorage(ctx, db); err != nil {
		client.Close()
		return nil, fmt.Errorf("drop player storage: %w", err)
	}
	if err := client.Schema.Create(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("migrate database schema: %w", err)
	}
	if err := migrateSubscriptions(ctx, db); err != nil {
		client.Close()
		return nil, fmt.Errorf("migrate subscriptions: %w", err)
	}
	if err := createTaskHistoryIndexes(ctx, db); err != nil {
		client.Close()
		return nil, err
	}

	return &Store{Client: client, db: db}, nil
}

func migrateSubscriptions(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='monitors'").Scan(&count)
	if err != nil || count == 0 {
		return err
	}
	_, err = db.ExecContext(ctx, `
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
func dropPlayerStorage(ctx context.Context, db *sql.DB) error {
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
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?",
			column.table, column.name).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			statements = append(statements, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", column.table, column.name))
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (store *Store) Ping(ctx context.Context) error {
	return store.db.PingContext(ctx)
}

func (store *Store) Close() error {
	return store.Client.Close()
}

func sqliteDSN(databasePath string) string {
	path := filepath.ToSlash(databasePath)
	if volume := filepath.VolumeName(databasePath); volume != "" {
		path = "/" + path
	}

	uri := url.URL{Scheme: "file", Path: path}
	query := uri.Query()
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "journal_mode(WAL)")
	// Reserve the writer before a transaction reads. Scan transactions and
	// offline task updates must not race while upgrading a WAL read snapshot.
	query.Set("_txlock", "immediate")
	uri.RawQuery = query.Encode()
	return uri.String()
}
