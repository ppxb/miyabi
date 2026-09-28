package monitor

import (
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

func TestNextMovieCheckPolicy(t *testing.T) {
	now := time.Date(2026, 9, 18, 10, 30, 0, 0, time.Local)
	at := func(month time.Month, day int) time.Time { return time.Date(2026, month, day, 4, 0, 0, 0, time.Local) }
	for _, test := range []struct {
		name    string
		release string
		want    time.Time
		stale   bool
	}{
		{"far before release polls in three days at the check time", "2026-10-30", at(9, 21), false},
		{"just before release polls on release day", "2026-09-20", at(9, 20), false},
		{"release day polls tomorrow", "2026-09-18", at(9, 19), false},
		{"day 30 after release still polls", "2026-08-19", at(9, 19), false},
		{"day 31 after release is stale", "2026-08-18", time.Time{}, true},
		{"missing date counts from creation", "", at(9, 19), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			next, stale := nextMovieCheck(now, test.release, now.AddDate(0, 0, -3), "04:00")
			if stale != test.stale || !next.Equal(test.want) {
				t.Fatalf("next=%v stale=%v, want next=%v stale=%v", next, stale, test.want, test.stale)
			}
		})
	}
	if next, _ := nextMovieCheck(now, "2026-09-18", now, "23:00"); !next.Equal(time.Date(2026, 9, 18, 23, 0, 0, 0, time.Local)) {
		t.Fatalf("a later check time today is still today, got %v", next)
	}
}

func TestNextDaily(t *testing.T) {
	now := time.Date(2026, 9, 18, 10, 30, 0, 0, time.Local)
	if got := nextDaily(now, "11:00"); !got.Equal(time.Date(2026, 9, 18, 11, 0, 0, 0, time.Local)) {
		t.Fatalf("later today expected, got %v", got)
	}
	if got := nextDaily(now, "04:00"); !got.Equal(time.Date(2026, 9, 19, 4, 0, 0, 0, time.Local)) {
		t.Fatalf("tomorrow expected, got %v", got)
	}
	if got := nextDaily(now, "garbage"); !got.Equal(time.Date(2026, 9, 19, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("invalid time must fall back to the default, got %v", got)
	}
}

func ids(movies []domain.Movie) []string {
	result := make([]string, len(movies))
	for i, movie := range movies {
		result[i] = movie.ID
	}
	return result
}

func TestActorCursor(t *testing.T) {
	today := "2026-09-18"
	page := []domain.Movie{
		{ID: "upcoming", ReleaseDate: "2026-12-01"},
		{ID: "latest", ReleaseDate: "2026-09-01"},
		{ID: "older", ReleaseDate: "2026-06-01"},
	}
	cursor := snapshotCursor(page, today)
	if cursor.LatestReleaseDate != "2026-09-01" {
		t.Fatalf("watermark must ignore unreleased titles, got %s", cursor.LatestReleaseDate)
	}
	if got := cursor.SeenMovieIDs; len(got) != 3 || got[0] != "older" || got[2] != "upcoming" {
		t.Fatalf("seen IDs must be stored oldest first, got %v", got)
	}
	if works := cursor.newWorks(page); len(works) != 0 {
		t.Fatalf("the baseline page has no new works, got %v", ids(works))
	}

	next := append([]domain.Movie{
		{ID: "brand-new", ReleaseDate: "2026-09-20"},
		{ID: "announced", ReleaseDate: "2026-10-15"}, // dated before the seen upcoming title, still new
		{ID: "late-indexed", ReleaseDate: "2026-08-20"},
		{ID: "backfill", ReleaseDate: "2010-01-01"},
		{ID: "undated"},
	}, page...)
	works := ids(cursor.newWorks(next))
	if len(works) != 4 || works[0] != "brand-new" || works[1] != "announced" || works[2] != "late-indexed" || works[3] != "undated" {
		t.Fatalf("unexpected new works %v", works)
	}
	advanced := cursor.advance(next, today)
	if advanced.LatestReleaseDate != "2026-09-01" {
		t.Fatalf("future and older dates must not move the watermark, got %s", advanced.LatestReleaseDate)
	}
	if works := advanced.newWorks(next); len(works) != 0 {
		t.Fatalf("everything on the page is now seen, got %v", ids(works))
	}

	many := make([]domain.Movie, seenMovieLimit+50)
	for i := range many {
		many[i] = domain.Movie{ID: string(rune('a'+i%26)) + string(rune('0'+i/26)), ReleaseDate: "2020-01-01"}
	}
	bounded := ActorCursor{}.advance(many, today)
	if len(bounded.SeenMovieIDs) != seenMovieLimit || bounded.SeenMovieIDs[seenMovieLimit-1] != many[0].ID {
		t.Fatalf("seen list must keep the newest %d IDs, got %d ending in %s", seenMovieLimit, len(bounded.SeenMovieIDs), bounded.SeenMovieIDs[len(bounded.SeenMovieIDs)-1])
	}
}

func TestActorCursor_Initialization(t *testing.T) {
	var empty ActorCursor
	if empty.isInitialized() {
		t.Fatal("empty cursor should not be initialized")
	}

	snapEmpty := snapshotCursor(nil, "2026-09-18")
	if !snapEmpty.isInitialized() || !snapEmpty.Initialized {
		t.Fatal("snapshotCursor on empty page must mark initialized")
	}

	encoded := snapEmpty.encode()
	decoded := decodeCursor(encoded)
	if !decoded.isInitialized() || !decoded.Initialized {
		t.Fatal("decoded cursor must preserve initialized flag")
	}

	legacyWithSeen := decodeCursor(`{"seen_movie_ids":["m1"]}`)
	if !legacyWithSeen.isInitialized() {
		t.Fatal("legacy cursor with seen IDs should be recognized as initialized")
	}

	legacyWithDate := decodeCursor(`{"latest_release_date":"2026-09-01"}`)
	if !legacyWithDate.isInitialized() {
		t.Fatal("legacy cursor with latest release date should be recognized as initialized")
	}
}

