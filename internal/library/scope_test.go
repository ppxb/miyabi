package library

import (
	"context"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestMoviesAggregateConfiguredSources(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	enabled := domain.LibrarySource{
		AccountID: "A",
		Directory: domain.LibraryDirectory{ID: "10", Name: "Movies", Path: "/Movies"},
	}
	lib := New(store.Client, nil, tasks.NewService(store.Client, tasks.NewRegistry()), nil, Options{
		Sources: func(context.Context) ([]domain.LibrarySource, error) {
			return []domain.LibrarySource{enabled}, nil
		},
	})

	createMovie := func(code, accountID, rootID, fileID string) int {
		film := store.Client.Movie.Create().SetCode(code).SaveX(ctx)
		store.Client.File.Create().SetFileID(fileID).SetName(code + ".mp4").SetSize(1024).
			SetAccountID(accountID).SetRootID(rootID).SetMovieID(film.ID).SaveX(ctx)
		return film.ID
	}
	inSource := createMovie("ABP-001", "A", "10", "file-a")
	inOther := createMovie("ABP-002", "B", "20", "file-b")
	inLocal := createMovie("ABP-003", domain.LocalAccountID, domain.LocalAccountID, "file-local")

	page, err := lib.Movies(ctx, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("total = %d, want 2", page.Total)
	}
	got := make(map[int]bool, len(page.Movies))
	for _, item := range page.Movies {
		got[item.ID] = true
	}
	if !got[inSource] || !got[inLocal] {
		t.Fatalf("missing expected movies: %v", got)
	}
	if got[inOther] {
		t.Fatal("movie from a non-enabled source leaked into the library")
	}
}
