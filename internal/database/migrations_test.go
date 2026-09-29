package database

import (
	"database/sql"
	"encoding/json/jsontext"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/ent/setting"
)

func setMigrationVersion(t *testing.T, store *Store, version int) {
	t.Helper()
	if _, err := store.db.ExecContext(t.Context(), fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		t.Fatal(err)
	}
}

func migrationVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var version int
	if err := db.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

func TestOpenMigratesViewedMoviesAndSkipsCompletedSteps(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if store != nil {
			_ = store.Close()
		}
	})
	if got := migrationVersion(t, store.db); got != len(migrations) {
		t.Fatalf("fresh version=%d", got)
	}
	oldTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	store.Client.ViewedMovie.Create().SetJavdbID("existing").SetViewedAt(oldTime).SaveX(ctx)
	store.Client.Setting.Create().SetKey(viewedMoviesLegacySetting).
		SetValue(jsontext.Value(`{"ids":[" existing ","legacy-1","legacy-1"," ","legacy-2"]}`)).SaveX(ctx)
	setMigrationVersion(t, store, 0)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := migrationVersion(t, store.db); got != len(migrations) {
		t.Fatalf("upgraded version=%d", got)
	}
	rows := store.Client.ViewedMovie.Query().AllX(ctx)
	if len(rows) != 3 {
		t.Fatalf("viewed rows=%d", len(rows))
	}
	for _, row := range rows {
		if row.JavdbID == "existing" && !row.ViewedAt.Equal(oldTime) {
			t.Fatal("overwrote existing history timestamp")
		}
	}
	if store.Client.Setting.Query().Where(setting.Key(viewedMoviesLegacySetting)).ExistX(ctx) {
		t.Fatal("legacy history not removed")
	}
	// Invalid legacy-looking data would fail if a completed migration ran again.
	store.Client.Setting.Create().SetKey(viewedMoviesLegacySetting).SetValue(jsontext.Value(`{"ids":123}`)).SaveX(ctx)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, dir)
	if err != nil {
		t.Fatalf("completed migration reran: %v", err)
	}
	if count := store.Client.ViewedMovie.Query().CountX(ctx); count != 3 {
		t.Fatalf("reopen changed history: %d", count)
	}
}

func TestMigrationFailureRollsBackAndResumes(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	// Fail after an insert, when removing the source record, to exercise the
	// transaction boundary around both data changes and the version update.
	store.Client.Setting.Create().SetKey(viewedMoviesLegacySetting).SetValue(jsontext.Value(`{"ids":["legacy"]}`)).SaveX(ctx)
	setMigrationVersion(t, store, 4)
	if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER fail_viewed_cleanup BEFORE DELETE ON settings WHEN OLD.key = 'browse.viewed_movies' BEGIN SELECT RAISE(ABORT, 'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if failed, err := Open(ctx, dir); err == nil {
		failed.Close()
		t.Fatal("migration failure was ignored")
	}
	db, err := sql.Open("sqlite", sqliteDSN(filepath.Join(dir, "miyabi.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if version := migrationVersion(t, db); version != 4 {
		t.Fatalf("failed version advanced to %d", version)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM viewed_movies").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rows escaped rollback: %d %v", count, err)
	}
	if _, err := db.ExecContext(ctx, "DROP TRIGGER fail_viewed_cleanup"); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, dir)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	defer store.Close()
	if migrationVersion(t, store.db) != len(migrations) || store.Client.ViewedMovie.Query().CountX(ctx) != 1 {
		t.Fatal("resume did not complete")
	}
}

func TestOpenRejectsFutureVersionBeforeChangingSchema(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", sqliteDSN(filepath.Join(dir, "miyabi.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(t.Context(), fmt.Sprintf("PRAGMA user_version = %d; CREATE TABLE watch_histories (id INTEGER)", len(migrations)+1)); err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.Context(), dir)
	if err == nil {
		store.Close()
		t.Fatal("accepted future version")
	}
	if !strings.Contains(err.Error(), "unsupported database migration version") {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("schema was changed: %d %v", count, err)
	}
}
