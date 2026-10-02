package library

import (
	"context"
	stdsql "database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
)

type listingQueryDriver struct {
	dialect.Driver
	query string
	args  []any
}

func (d *listingQueryDriver) Query(ctx context.Context, query string, args, value any) error {
	if strings.Contains(query, "`movies`") && strings.Contains(query, "ORDER BY") {
		d.query, d.args = query, args.([]any)
	}
	return d.Driver.Query(ctx, query, args, value)
}

func TestLibraryListingUsesSortIndexAndKeepsScopeAndOrder(t *testing.T) {
	ctx := t.Context()
	db, err := stdsql.Open("sqlite", filepath.Join(t.TempDir(), "listing.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	driver := &listingQueryDriver{Driver: sql.OpenDB(dialect.SQLite, db)}
	client := ent.NewClient(ent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	base := time.Now()
	var ids []int
	for i, entry := range []struct {
		account, root string
		created       time.Time
	}{
		{domain.LocalAccountID, "local", base.Add(-time.Hour)},
		{domain.LocalAccountID, "local", base},
		{domain.LocalAccountID, "local", base},
		{"account", "root", base.Add(time.Hour)},
		{"other-account", "root", base.Add(2 * time.Hour)},
		{"account", "other-root", base.Add(3 * time.Hour)},
	} {
		film := client.Movie.Create().SetCode(fmt.Sprintf("TEST-%03d", i)).SetCreatedAt(entry.created).SaveX(ctx)
		ids = append(ids, film.ID)
		client.File.Create().SetFileID(fmt.Sprint(i)).SetName(film.Code + ".mp4").SetSize(1).
			SetAccountID(entry.account).SetRootID(entry.root).SetMovieID(film.ID).ExecX(ctx)
	}
	client.Movie.Create().SetCode("ORPHAN-001").SetCreatedAt(base.Add(4 * time.Hour)).ExecX(ctx)
	client.File.Create().SetFileID("second-part").SetName("TEST-002-part2.mp4").SetSize(1).
		SetAccountID(domain.LocalAccountID).SetRootID("local").SetMovieID(ids[2]).ExecX(ctx)
	service := &Service{database: client}
	for _, mounted := range []bool{false, true} {
		want := []int{ids[2], ids[1], ids[0]}
		if mounted {
			service.drive = newMountedDrive(t, client, &panStub{}, domain.LibrarySource{
				AccountID: "account", Directory: domain.LibraryDirectory{ID: "root", Name: "Movies", Path: "/Movies"},
			})
			want = append([]int{ids[3]}, want...)
		}
		var got []int
		for page := 1; page <= (len(want)+1)/2; page++ {
			result, err := service.Movies(ctx, page, 2)
			if err != nil || result.Total != len(want) || result.HasMore != (page*2 < len(want)) {
				t.Fatalf("mounted=%t page=%d result=%+v err=%v", mounted, page, result, err)
			}
			for _, film := range result.Movies {
				got = append(got, film.ID)
			}
		}
		if !slices.Equal(got, want) {
			t.Fatalf("mounted=%t order=%v want=%v", mounted, got, want)
		}
		if driver.query == "" {
			t.Fatal("listing query was not captured")
		}
		rows, err := db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+driver.query, driver.args...)
		if err != nil {
			t.Fatal(err)
		}
		var plan []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			plan = append(plan, detail)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(plan, "; ")
		if !strings.Contains(joined, "movie_created_at_id") || strings.Contains(joined, "TEMP B-TREE FOR ORDER BY") {
			t.Fatalf("mounted=%t listing missed sort index: %s", mounted, joined)
		}
	}
}
