package scan

import (
	"testing"

	"github.com/ppxb/miyabi/internal/database"
)

func TestIndexMoviesBindsEquivalentNumbers(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db := store.Client

	decorated := db.Movie.Create().SetCode("259LUXU-1899").SaveX(ctx)
	unscraped := db.Movie.Create().SetCode("GANA-3458").SaveX(ctx)
	scraped := db.Movie.Create().SetCode("200GANA-3458").SetJavdbID("gana").SaveX(ctx)

	tx, err := db.Tx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	codes := []string{"LUXU-1899", "GANA-3458", "326IHD-005", "IHD-005", "SSIS-001"}
	matched, err := indexMovies(ctx, tx, codes)
	if err != nil {
		t.Fatal(err)
	}
	if matched["LUXU-1899"] != decorated.ID {
		t.Errorf("undecorated file did not bind to the decorated record: %v", matched)
	}
	if matched["GANA-3458"] != scraped.ID || matched["GANA-3458"] == unscraped.ID {
		t.Errorf("scraped record did not win over the exact spelling: %v", matched)
	}
	if matched["326IHD-005"] == 0 || matched["326IHD-005"] != matched["IHD-005"] {
		t.Errorf("equivalent new numbers were split across records: %v", matched)
	}
	if code := tx.Movie.GetX(ctx, matched["IHD-005"]).Code; code != "IHD-005" {
		t.Errorf("shared record code = %q, want the relaxed spelling", code)
	}
	if matched["SSIS-001"] == 0 || matched["SSIS-001"] == matched["IHD-005"] {
		t.Errorf("unrelated number was not indexed separately: %v", matched)
	}
}
