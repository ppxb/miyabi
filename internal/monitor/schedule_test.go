package monitor

import (
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

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
	if empty.Initialized {
		t.Fatal("empty cursor should not be initialized")
	}

	snapEmpty := snapshotCursor(nil, "2026-09-18")
	if !snapEmpty.Initialized {
		t.Fatal("snapshotCursor on empty page must mark initialized")
	}

	encoded := snapEmpty.encode()
	decoded := decodeCursor(encoded)
	if !decoded.Initialized {
		t.Fatal("decoded cursor must preserve initialized flag")
	}
}
