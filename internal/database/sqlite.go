package database

import (
	"context"
	"database/sql"
	"fmt"
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
	client := ent.NewClient(ent.Driver(driver))
	if err := client.Schema.Create(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("migrate database schema: %w", err)
	}
	if err := createTaskHistoryIndexes(ctx, db); err != nil {
		client.Close()
		return nil, err
	}

	return &Store{Client: client, db: db}, nil
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
