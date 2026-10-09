package library

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/viewedmovie"
)

func viewedFixture(t testing.TB, count int, viewedAt time.Time) *Service {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := ent.WithTx(t.Context(), store.Client, func(tx *ent.Tx) error {
		for start := 0; start < count; start += 500 {
			var rows []*ent.ViewedMovieCreate
			for i := start; i < min(start+500, count); i++ {
				rows = append(rows, tx.ViewedMovie.Create().SetJavdbID(fmt.Sprintf("seed-%d", i)).SetViewedAt(viewedAt))
			}
			if err := tx.ViewedMovie.CreateBulk(rows...).Exec(t.Context()); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return New(store.Client, nil, nil, nil, Options{})
}

func TestViewedMovies_CRUDAndOrdering(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	svc := New(store.Client, nil, nil, nil, Options{})

	// Initially empty
	ids, err := svc.ViewedMovieIDs(ctx)
	if err != nil {
		t.Fatalf("ViewedMovieIDs() error = %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected empty ids, got %v", ids)
	}

	// Empty / whitespace ignored
	if err := svc.AddViewedMovieIDs(ctx, []string{"", "   "}); err != nil {
		t.Fatalf("AddViewedMovieIDs() empty error = %v", err)
	}
	ids, err = svc.ViewedMovieIDs(ctx)
	if err != nil || len(ids) != 0 {
		t.Fatalf("expected empty ids after blank add, got %v", ids)
	}

	// Add batch
	if err := svc.AddViewedMovieIDs(ctx, []string{"id-1", " id-2 ", "id-1", "id-3"}); err != nil {
		t.Fatalf("AddViewedMovieIDs() error = %v", err)
	}
	ids, err = svc.ViewedMovieIDs(ctx)
	if err != nil {
		t.Fatalf("ViewedMovieIDs() error = %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 ids, got %v", ids)
	}

	// Re-add id-1 should move it to the front
	if err := svc.AddViewedMovieIDs(ctx, []string{"id-1"}); err != nil {
		t.Fatalf("AddViewedMovieIDs() re-add error = %v", err)
	}
	ids, err = svc.ViewedMovieIDs(ctx)
	if err != nil {
		t.Fatalf("ViewedMovieIDs() error = %v", err)
	}
	if len(ids) != 3 || ids[0] != "id-1" {
		t.Fatalf("expected id-1 at front, got %v", ids)
	}
}

func TestViewedMoviesPruningBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name         string
		count, added int
	}{
		{"below limit", 10, 1},
		{"exact limit", maxViewedMovies - 2, 2},
		{"one over limit", maxViewedMovies, 1},
		{"maximum API batch", maxViewedMovies, 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := viewedFixture(t, tc.count, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
			input := make([]string, tc.added)
			for i := range input {
				input[i] = fmt.Sprintf("new-%d", i)
			}
			if err := svc.AddViewedMovieIDs(t.Context(), input); err != nil {
				t.Fatal(err)
			}
			want := slices.Clone(input)
			slices.Reverse(want)
			for i := tc.count - 1; i >= max(0, tc.count+tc.added-maxViewedMovies); i-- {
				want = append(want, fmt.Sprintf("seed-%d", i))
			}
			got, err := svc.ViewedMovieIDs(t.Context())
			if err != nil || !slices.Equal(got, want) {
				t.Fatalf("pruned history differs from newest-first ordering: got=%d want=%d error=%v", len(got), len(want), err)
			}
			if count := svc.database.ViewedMovie.Query().CountX(t.Context()); count != len(want) {
				t.Fatalf("stored history has %d rows, want %d", count, len(want))
			}
		})
	}
}

func TestViewedMoviesPruningKeepsRefreshedOldestEntry(t *testing.T) {
	svc := viewedFixture(t, maxViewedMovies, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	if err := svc.AddViewedMovieIDs(t.Context(), []string{"seed-0", " new-a ", "new-a", "", "new-b"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"new-b", "new-a", "seed-0"}
	for i := maxViewedMovies - 1; i >= 3; i-- {
		want = append(want, fmt.Sprintf("seed-%d", i))
	}
	got, err := svc.ViewedMovieIDs(t.Context())
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("refresh or equal-time eviction order changed: got=%d want=%d error=%v", len(got), len(want), err)
	}
	if count := svc.database.ViewedMovie.Query().CountX(t.Context()); count != maxViewedMovies {
		t.Fatalf("unexpected stored row count: %d", count)
	}
}

func TestViewedMoviesAdvanceTimeAfterClockRollback(t *testing.T) {
	future := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := viewedFixture(t, 1, future)
	for i, id := range []string{"new", "seed-0"} {
		if err := svc.AddViewedMovieIDs(t.Context(), []string{id}); err != nil {
			t.Fatal(err)
		}
		row := svc.database.ViewedMovie.Query().Where(viewedmovie.JavdbID(id)).OnlyX(t.Context())
		want := future.Add(time.Duration(i+1) * time.Microsecond)
		if !row.ViewedAt.Equal(want) {
			t.Fatalf("view timestamp=%s, want %s", row.ViewedAt, want)
		}
		ids, err := svc.ViewedMovieIDs(t.Context())
		if err != nil || len(ids) != 2 || ids[0] != id {
			t.Fatalf("clock rollback displaced latest view: %v %v", ids, err)
		}
	}
}

func TestViewedMoviesConcurrentWritesStayBounded(t *testing.T) {
	svc := viewedFixture(t, maxViewedMovies-1, time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
	start := make(chan struct{})
	var group sync.WaitGroup
	for i := range 8 {
		group.Go(func() {
			<-start
			if err := svc.AddViewedMovieIDs(t.Context(), []string{"shared", fmt.Sprintf("new-%d", i)}); err != nil {
				t.Error(err)
			}
		})
	}
	close(start)
	group.Wait()
	if count := svc.database.ViewedMovie.Query().CountX(t.Context()); count != maxViewedMovies {
		t.Fatalf("concurrent writes left %d rows", count)
	}
	rows := svc.database.ViewedMovie.Query().Where(viewedmovie.JavdbIDHasPrefix("new-")).AllX(t.Context())
	times := make(map[time.Time]bool)
	var latest time.Time
	for _, row := range rows {
		times[row.ViewedAt] = true
		if row.ViewedAt.After(latest) {
			latest = row.ViewedAt
		}
	}
	if len(rows) != 8 || len(times) != 8 {
		t.Fatalf("concurrent writes lost rows or monotonic times: rows=%d timestamps=%d", len(rows), len(times))
	}
	shared := svc.database.ViewedMovie.Query().Where(viewedmovie.JavdbID("shared")).OnlyX(t.Context())
	if !shared.ViewedAt.Equal(latest) {
		t.Fatal("shared view did not retain the latest update")
	}
	for i := range 8 {
		if svc.database.ViewedMovie.Query().Where(viewedmovie.JavdbID(fmt.Sprintf("seed-%d", i))).ExistX(t.Context()) {
			t.Fatalf("oldest seed-%d was not evicted", i)
		}
	}
}

func TestViewedMoviesPruningFailureRollsBackWrites(t *testing.T) {
	before := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := viewedFixture(t, maxViewedMovies, before)
	pruneErr := errors.New("pruning failed")
	svc.database.ViewedMovie.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if mutation.Op().Is(ent.OpDelete) {
				return nil, pruneErr
			}
			return next.Mutate(ctx, mutation)
		})
	})
	if err := svc.AddViewedMovieIDs(t.Context(), []string{"seed-0", "new"}); !errors.Is(err, pruneErr) {
		t.Fatalf("pruning error was not returned: %v", err)
	}
	if count := svc.database.ViewedMovie.Query().CountX(t.Context()); count != maxViewedMovies {
		t.Fatalf("failed pruning left %d rows", count)
	}
	if svc.database.ViewedMovie.Query().Where(viewedmovie.JavdbID("new")).ExistX(t.Context()) {
		t.Fatal("failed pruning retained an uncommitted view")
	}
	row := svc.database.ViewedMovie.Query().Where(viewedmovie.JavdbID("seed-0")).OnlyX(t.Context())
	if !row.ViewedAt.Equal(before) {
		t.Fatal("failed pruning retained the timestamp update")
	}
}
