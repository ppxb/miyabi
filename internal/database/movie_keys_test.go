package database

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
)

func TestMovieMatchKeyUpgradePreservesRecordsAndAssociations(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if store != nil {
			_ = store.Close()
		}
	})
	film := store.Client.Movie.Create().SetCode("259LUXU-01899").SetJavdbID("identity").
		SetTitle("Preserved metadata").SetFanarts([]string{"/fanart.jpg"}).SaveX(ctx)
	store.Client.Movie.Create().SetCode("LUXU-1899").SaveX(ctx)
	store.Client.File.Create().SetFileID("video").SetName("259LUXU-01899.mp4").SetSize(1024).
		SetAccountID("account").SetRootID("root").SetMovie(film).ExecX(ctx)
	snapshot := func() []byte {
		t.Helper()
		records := store.Client.Movie.Query().WithFiles().Order(movie.ByID()).AllX(ctx)
		data, err := json.Marshal(records)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	before := snapshot()
	// Model version 8, which had neither the lookup column nor its index.
	for _, statement := range []string{
		"DROP INDEX movie_canonical_code",
		"ALTER TABLE movies DROP COLUMN canonical_code",
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	setMigrationVersion(t, store, 8)
	for range 2 {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		store, err = Open(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := migrationVersion(t, store.db); got != len(migrations) {
			t.Fatalf("migration version=%d", got)
		}
		if after := snapshot(); !bytes.Equal(before, after) {
			t.Fatalf("migration changed movie data or associations:\nbefore=%s\nafter=%s", before, after)
		}
		if count := store.Client.Movie.Query().Where(movie.CanonicalCodeEQ("LUXU-1899")).CountX(ctx); count != 2 {
			t.Fatalf("backfill lost an alias or merged records: %d", count)
		}
		plan := queryPlan(t, store, "SELECT id, code, canonical_code, javdb_id FROM movies WHERE canonical_code IN (?, ?) ORDER BY id", "LUXU-1899", "ABC-123")
		if !strings.Contains(plan, "SEARCH movies USING INDEX movie_canonical_code (canonical_code=?)") {
			t.Fatalf("matching query missed the index: %s", plan)
		}
		t.Log(plan)
	}
}

func TestMovieMatchKeyFollowsWrites(t *testing.T) {
	ctx := t.Context()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db := store.Client
	check := func(id int, code, key string) {
		t.Helper()
		got := db.Movie.GetX(ctx, id)
		if got.Code != code || got.CanonicalCode != key {
			t.Fatalf("movie %d: code=%q key=%q, want code=%q key=%q", id, got.Code, got.CanonicalCode, code, key)
		}
	}
	film := db.Movie.Create().SetCode("259LUXU-01899").SetJavdbID("identity").SaveX(ctx)
	check(film.ID, "259LUXU-01899", "LUXU-1899")
	created := db.Movie.CreateBulk(db.Movie.Create().SetCode("ABC-00123"), db.Movie.Create().SetCode("XYZ-000")).SaveX(ctx)
	check(created[0].ID, "ABC-00123", "ABC-123")
	check(created[1].ID, "XYZ-000", "XYZ-0")
	db.Movie.UpdateOneID(film.ID).SetCode("CURRENT-001").ExecX(ctx)
	check(film.ID, "CURRENT-001", "CURRENT-1")
	db.Movie.Update().Where(movie.IDEQ(film.ID)).SetCode("200GANA-3458").ExecX(ctx)
	check(film.ID, "200GANA-3458", "GANA-3458")
	db.Movie.UpdateOneID(film.ID).SetTitle("Metadata only").ExecX(ctx)
	check(film.ID, "200GANA-3458", "GANA-3458")
	if err := db.Movie.Create().SetCode("FC2-1234567").SetJavdbID("identity").
		OnConflictColumns(movie.FieldJavdbID).UpdateNewValues().Exec(ctx); err != nil {
		t.Fatal(err)
	}
	check(film.ID, "FC2-1234567", "FC2-PPV-1234567")
	if err := db.Movie.CreateBulk(db.Movie.Create().SetCode("ABC-00001").SetJavdbID("identity")).
		OnConflictColumns(movie.FieldJavdbID).UpdateNewValues().Exec(ctx); err != nil {
		t.Fatal(err)
	}
	check(film.ID, "ABC-00001", "ABC-1")
	tx, err := db.Tx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	changed := tx.Movie.UpdateOneID(film.ID).SetCode("ROLLBACK-001").SaveX(ctx)
	if changed.CanonicalCode != "ROLLBACK-1" {
		t.Fatal("transaction did not inherit the movie key hook")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	check(film.ID, "ABC-00001", "ABC-1")
	if err := db.Movie.UpdateOneID(film.ID).SetCode(created[0].Code).Exec(ctx); !ent.IsConstraintError(err) {
		t.Fatalf("expected unique code conflict, got %v", err)
	}
	check(film.ID, "ABC-00001", "ABC-1")
}
