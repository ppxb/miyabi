package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/watchhistory"
)

func historyFilm(t *testing.T, library *LibraryService, source LibrarySource, code string) (*ent.Movie, *ent.File) {
	t.Helper()
	film := library.database.Movie.Create().SetCode(code).SetTitle("Title " + code).SetWatched(true).SaveX(t.Context())
	video := library.database.File.Create().SetFileID(code).SetName(code + ".mp4").SetSize(1 << 30).
		SetAccountID(source.AccountID).SetRootID(source.Directory.ID).SetMovie(film).SaveX(t.Context())
	return film, video
}

func TestWatchHistoryPaginatesRecentMoviesWithBoundedScopedQueries(t *testing.T) {
	library, _, payload := libraryFixture(t)
	ctx := t.Context()
	stamp := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	for i := range 23 {
		film, _ := historyFilm(t, library, payload.Source, fmt.Sprintf("ABP-%03d", i))
		library.database.WatchHistory.Create().SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).
			SetMovie(film).SetSessionID(uuid.NewString()).SetWatchedAt(stamp.Add(time.Duration(i) * time.Minute)).
			SetPosition(float64(i)).SetDuration(100).ExecX(ctx)
		// Multiple video files must not duplicate a movie in history or its count.
		library.database.File.Create().SetFileID(fmt.Sprintf("part-%d", i)).SetName("part.mp4").SetSize(1 << 30).
			SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).SetMovie(film).ExecX(ctx)
	}
	for i, source := range []LibrarySource{
		{AccountID: "other", Directory: payload.Source.Directory},
		{AccountID: payload.Source.AccountID, Directory: PanLibraryDirectory{ID: "other"}},
	} {
		film, _ := historyFilm(t, library, source, fmt.Sprintf("HIDDEN-%d", i))
		library.database.WatchHistory.Create().SetAccountID(source.AccountID).SetRootID(source.Directory.ID).
			SetMovie(film).SetSessionID(uuid.NewString()).ExecX(ctx)
	}
	queries := map[string]int{}
	library.database.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			queries[fmt.Sprintf("%T", query)]++
			return next.Query(ctx, query)
		})
	}))
	page, err := library.WatchHistory(ctx, 1)
	if err != nil || page.Source == nil || *page.Source != payload.Source || page.Total != 23 || !page.HasMore || len(page.Items) != 20 {
		t.Fatalf("incorrect first page: %+v, %v", page, err)
	}
	if page.Items[0].Code != "ABP-022" || page.Items[0].Position != 22 || page.Items[0].Duration != 100 || page.Items[19].Code != "ABP-003" {
		t.Fatalf("history lost progress or recent order: %+v", page.Items)
	}
	want := map[string]int{"*ent.SettingQuery": 1, "*ent.WatchHistoryQuery": 2, "*ent.MovieQuery": 1}
	if !reflect.DeepEqual(queries, want) {
		t.Fatalf("history loaded redundant data: %v", queries)
	}
	page, err = library.WatchHistory(ctx, 2)
	if err != nil || len(page.Items) != 3 || page.HasMore || page.Items[2].Code != "ABP-000" {
		t.Fatalf("incorrect final page: %+v, %v", page, err)
	}
	page, err = library.WatchHistory(ctx, 3)
	if err != nil || len(page.Items) != 0 || page.Total != 23 || page.Items == nil {
		t.Fatalf("out-of-range page lost its total or empty array: %+v, %v", page, err)
	}
	library.database.Setting.Delete().ExecX(ctx)
	page, err = library.WatchHistory(ctx, 1)
	if err != nil || page.Source != nil || page.Total != 0 || page.Items == nil || len(page.Items) != 0 {
		t.Fatalf("unmounted history must be empty: %+v, %v", page, err)
	}
}

func TestWatchProgressPreservesResumeAndRejectsStaleSessionsAndVersions(t *testing.T) {
	library, _, payload := libraryFixture(t)
	ctx := t.Context()
	film, video := historyFilm(t, library, payload.Source, "ABP-001")
	session, err := library.MarkWatched(ctx, film.ID)
	if err != nil {
		t.Fatal(err)
	}
	before := library.tasks.Revisions()
	progress := WatchProgress{SessionID: session.SessionID, FileID: video.FileID, Position: 120, Duration: 600, Version: 2}
	if err := library.SaveWatchProgress(ctx, session.ID, progress); err != nil {
		t.Fatal(err)
	}
	after := library.tasks.Revisions()
	if after.History != before.History+1 || after.Library != before.Library || after.Offline != before.Offline {
		t.Fatal("saving progress refreshed unrelated library or offline data")
	}
	progress.Version, progress.Position = 1, 300
	if err := library.SaveWatchProgress(ctx, session.ID, progress); err != nil {
		t.Fatal(err)
	}
	if row := library.database.WatchHistory.GetX(ctx, session.ID); row.Position != 120 || row.ProgressVersion != 2 || library.tasks.Revisions() != after {
		t.Fatalf("an older request overwrote progress: %+v", row)
	}
	// Seeking backward is valid when it belongs to a newer request.
	progress.Version, progress.Position = 3, 30
	if err := library.SaveWatchProgress(ctx, session.ID, progress); err != nil {
		t.Fatal(err)
	}
	reopened, err := library.MarkWatched(ctx, film.ID)
	if err != nil || reopened.ID != session.ID || reopened.SessionID == session.SessionID || reopened.Position != 30 || reopened.Duration != 600 || reopened.FileID != video.FileID {
		t.Fatalf("reopening lost resume information: %+v, %v", reopened, err)
	}
	progress.Version, progress.Position = 100, 550
	if err := library.SaveWatchProgress(ctx, session.ID, progress); err != nil {
		t.Fatal(err)
	}
	if row := library.database.WatchHistory.GetX(ctx, session.ID); row.Position != 30 || row.ProgressVersion != 0 {
		t.Fatal("a previous playback session replaced the new session")
	}
	progress.SessionID, progress.Version, progress.Position = reopened.SessionID, 1, 601
	if err := library.SaveWatchProgress(ctx, session.ID, progress); err != nil {
		t.Fatal(err)
	}
	if row := library.database.WatchHistory.GetX(ctx, session.ID); row.Position != 600 {
		t.Fatal("completed playback escaped its duration")
	}
}

func TestWatchProgressValidatesFilesNumbersAndMountedSource(t *testing.T) {
	library, _, payload := libraryFixture(t)
	ctx := t.Context()
	film, video := historyFilm(t, library, payload.Source, "ABP-001")
	_, otherVideo := historyFilm(t, library, payload.Source, "ABP-002")
	session, err := library.MarkWatched(ctx, film.ID)
	if err != nil {
		t.Fatal(err)
	}
	progress := WatchProgress{SessionID: session.SessionID, FileID: video.FileID, Position: 1, Duration: 600, Version: 1}
	for _, mutate := range []func(*WatchProgress){
		func(p *WatchProgress) { p.Position = -1 }, func(p *WatchProgress) { p.Position = math.NaN() },
		func(p *WatchProgress) { p.Duration = math.Inf(1) }, func(p *WatchProgress) { p.Duration = 0 },
		func(p *WatchProgress) { p.Version = 0 },
	} {
		invalid := progress
		mutate(&invalid)
		if err := library.SaveWatchProgress(ctx, session.ID, invalid); !errors.Is(err, ErrInvalidWatchProgress) {
			t.Fatalf("invalid progress was accepted: %+v, %v", invalid, err)
		}
	}
	progress.FileID = otherVideo.FileID
	if err := library.SaveWatchProgress(ctx, session.ID, progress); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a file from another movie was accepted: %v", err)
	}
	progress.FileID = video.FileID
	if err := saveSetting(ctx, library.database, panDirectorySetting, panLibraryDirectory{
		AccountID: "other", PanLibraryDirectory: payload.Source.Directory,
	}); err != nil {
		t.Fatal(err)
	}
	if err := library.SaveWatchProgress(ctx, session.ID, progress); !ent.IsNotFound(err) {
		t.Fatalf("an inactive source accepted progress: %v", err)
	}
	if row := library.database.WatchHistory.GetX(ctx, session.ID); row.Position != 0 || row.ProgressVersion != 0 {
		t.Fatal("rejected progress mutated the stored record")
	}
}

func TestClearingHistoryPreservesMoviesAndCannotBeUndoneByLateProgress(t *testing.T) {
	library, _, payload := libraryFixture(t)
	ctx := t.Context()
	first, video := historyFilm(t, library, payload.Source, "ABP-001")
	second, _ := historyFilm(t, library, payload.Source, "ABP-002")
	firstSession, err := library.MarkWatched(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondSession, err := library.MarkWatched(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	other := library.database.WatchHistory.Create().SetAccountID("other").SetRootID("10").SetMovie(first).
		SetSessionID(uuid.NewString()).SaveX(ctx)
	scope := WatchHistoryScope{AccountID: payload.Source.AccountID, DirectoryID: payload.Source.Directory.ID}
	if count, err := library.RemoveWatchHistory(ctx, scope, nil); err != nil || count != 0 {
		t.Fatalf("empty selection must not clear history: %d, %v", count, err)
	}
	if _, err := library.ClearWatchHistory(ctx, WatchHistoryScope{AccountID: "other", DirectoryID: "10"}); !errors.Is(err, ErrWatchHistorySourceChanged) {
		t.Fatalf("a stale clear operation was accepted: %v", err)
	}
	if count, err := library.RemoveWatchHistory(ctx, scope, []int{firstSession.ID, other.ID}); err != nil || count != 1 {
		t.Fatalf("selected deletion escaped its source: %d, %v", count, err)
	}
	progress := WatchProgress{SessionID: firstSession.SessionID, FileID: video.FileID, Position: 100, Duration: 600, Version: 1}
	if err := library.SaveWatchProgress(ctx, firstSession.ID, progress); !ent.IsNotFound(err) {
		t.Fatalf("late progress recreated a cleared record: %v", err)
	}
	if count, err := library.ClearWatchHistory(ctx, scope); err != nil || count != 1 {
		t.Fatalf("clear all did not remove the remaining record: %d, %v", count, err)
	}
	if library.database.WatchHistory.Query().CountX(ctx) != 1 || !library.database.WatchHistory.Query().Where(watchhistory.IDEQ(other.ID)).ExistX(ctx) {
		t.Fatal("clear all removed another account's history")
	}
	if !library.database.Movie.GetX(ctx, first.ID).Watched || library.database.File.Query().CountX(ctx) != 2 {
		t.Fatal("clearing history removed files or reset watched badges")
	}
	if library.database.WatchHistory.Query().Where(watchhistory.IDEQ(secondSession.ID)).ExistX(ctx) {
		t.Fatal("clear all left an active history record")
	}
	// Removing an orphan movie during scanning must cascade to history.
	library.database.File.Delete().Where(file.MovieIDEQ(first.ID)).ExecX(ctx)
	library.database.Movie.DeleteOneID(first.ID).ExecX(ctx)
	if library.database.WatchHistory.Query().CountX(ctx) != 0 {
		t.Fatal("removed movie left orphan history")
	}
}
