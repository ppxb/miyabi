package database

import (
	"strings"
	"testing"
	"time"
)

func TestWatchHistoryMigratesPreservesLegacyBadgesAndSurvivesReopen(t *testing.T) {
	ctx := t.Context()
	directory := t.TempDir()
	store, err := Open(ctx, directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if store != nil {
			_ = store.Close()
		}
	})
	film := store.Client.Movie.Create().SetCode("ABP-001").SetTitle("Existing title").SetWatched(true).SaveX(ctx)
	video := store.Client.File.Create().SetFileID("video").SetName("ABP-001.mp4").SetSize(1 << 30).
		SetAccountID("100").SetRootID("10").SetMovie(film).SaveX(ctx)
	// Existing databases have watched badges, but no timestamps or saved positions.
	if _, err := store.db.ExecContext(ctx, "DROP TABLE watch_histories"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, directory)
	if err != nil {
		t.Fatal(err)
	}
	if !store.Client.Movie.GetX(ctx, film.ID).Watched || store.Client.File.GetX(ctx, video.ID).MovieID == nil || store.Client.WatchHistory.Query().CountX(ctx) != 0 {
		t.Fatal("history migration lost existing data or fabricated past watch times")
	}
	stamp := time.Date(2026, 9, 11, 12, 30, 0, 0, time.UTC)
	history := store.Client.WatchHistory.Create().SetAccountID("100").SetRootID("10").SetMovieID(film.ID).
		SetSessionID("session").SetFileID(video.FileID).SetWatchedAt(stamp).SetPosition(125.5).SetDuration(600).
		SetProgressVersion(2).SaveX(ctx)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, directory)
	if err != nil {
		t.Fatal(err)
	}
	got := store.Client.WatchHistory.GetX(ctx, history.ID)
	if got.Position != 125.5 || got.Duration != 600 || got.FileID != video.FileID || got.ProgressVersion != 2 || !got.WatchedAt.Equal(stamp) {
		t.Fatalf("reopen lost saved playback progress: %+v", got)
	}
	rows, err := store.db.QueryContext(ctx, `EXPLAIN QUERY PLAN SELECT id FROM watch_histories
		WHERE account_id='100' AND root_id='10' ORDER BY watched_at DESC, id DESC LIMIT 20`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil || !strings.Contains(strings.Join(details, "\n"), "watchhistory_account_id_root_id_watched_at_id") {
		t.Fatalf("history query did not use its source/time index: %v, %v", details, err)
	}
}
