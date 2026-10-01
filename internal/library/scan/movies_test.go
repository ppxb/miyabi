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

	codes := []string{"LUXU-1899", "GANA-3458", "326IHD-005", "IHD-005", "SSIS-001", "SSIS-001",
		"259NEW-001", "999NEW-001", "ABC-00123", "ABC-123"}
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
	if matched["259NEW-001"] == 0 || matched["259NEW-001"] == matched["999NEW-001"] {
		t.Errorf("different distributors were merged through a shared lookup key: %v", matched)
	}
	if matched["ABC-00123"] == 0 || matched["ABC-00123"] != matched["ABC-123"] {
		t.Errorf("format-equivalent new numbers were split: %v", matched)
	}
}

func TestMatchMoviesPreservesRankingAndRejectsSharedFallbacks(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ids := make(map[string]int)
	for _, entry := range []struct{ code, javdbID string }{
		{"259LUXU-1899", ""}, {"999LUXU-1899", ""},
		{"GANA-3458", ""}, {"200GANA-3458", "gana"}, {"300GANA-3458", "other-gana"},
		{"1PONDO-060326-001", ""}, {"ABC-00123", ""}, {"FJIN-106A", ""},
		{"SCUTE-1575-ITSUKI", ""}, {"SSIS-589-02", ""},
		{"fc2_1234567", ""}, {"OLD-001", ""},
		{"042126_100", "date-film"}, {"TUSHYRAW.2026.09.27", "western-film"},
	} {
		builder := store.Client.Movie.Create().SetCode(entry.code)
		if entry.javdbID != "" {
			builder.SetJavdbID(entry.javdbID)
		}
		ids[entry.code] = builder.SaveX(ctx).ID
	}
	// A scrape can change both the number and the bucket used by the next scan.
	store.Client.Movie.UpdateOneID(ids["OLD-001"]).SetCode("CURRENT-001").ExecX(ctx)
	cases := []struct{ code, owner string }{
		{"LUXU-1899", "259LUXU-1899"}, // Equal rank keeps the lowest movie ID.
		{"999LUXU-1899", "999LUXU-1899"}, {"888LUXU-1899", ""},
		{"GANA-3458", "200GANA-3458"}, // Scraped beats an unscraped exact number.
		{"300GANA-3458", "300GANA-3458"},
		{"060326-001", "1PONDO-060326-001"}, {"CARIB-060326-001", ""},
		{"abc123", "ABC-00123"}, {"ABC-00123", "ABC-00123"},
		{"FJIN-106", ""}, {"FJIN-106B", ""}, {"FJIN-106A", "FJIN-106A"},
		{"SCUTE-1575", ""}, {"SCUTE-1575-ITSUKI", "SCUTE-1575-ITSUKI"},
		{"SSIS-589", ""}, {"SSIS-589-02", "SSIS-589-02"},
		{"FC2-PPV-1234567", "fc2_1234567"}, {"CURRENT-1", "OLD-001"},
		{"OLD-001", ""}, {"MISSING-001", ""}, {"", ""},
		{"PACOPACOMAMA-042126-100", "042126_100"}, {"042126-100", "042126_100"},
		{"TUSHYRAW.26.09.27", "TUSHYRAW.2026.09.27"}, {"TUSHYRAW.23.09.27", ""},
	}
	var codes []string
	for _, tt := range cases {
		codes = append(codes, tt.code)
	}
	tx, err := store.Client.Tx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	matched, err := MatchMovies(ctx, tx, codes)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases {
		if got := matched[tt.code]; got != ids[tt.owner] {
			t.Errorf("MatchMovies(%q)=%d, want %q (%d)", tt.code, got, tt.owner, ids[tt.owner])
		}
	}
}

func TestIndexMoviesReusesDateSpellingsAcrossScans(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	tx, err := store.Client.Tx(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	codes := []string{"042126_100", "PACOPACOMAMA-042126-100", "TUSHYRAW.26.09.27", "TUSHYRAW.2026.09.27"}
	for range 2 {
		matched, err := indexMovies(t.Context(), tx, codes)
		if err != nil {
			t.Fatal(err)
		}
		if matched[codes[0]] == 0 || matched[codes[0]] != matched[codes[1]] ||
			matched[codes[2]] == 0 || matched[codes[2]] != matched[codes[3]] {
			t.Fatalf("date spellings created duplicate movies: %v", matched)
		}
		if count := tx.Movie.Query().CountX(t.Context()); count != 2 {
			t.Fatalf("scan created %d movies, want 2", count)
		}
	}
}
