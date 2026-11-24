package playback

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
)

func TestStartMoviePlaysLocalFile(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	root := t.TempDir()
	rel := filepath.Join("sub", "ABP-001.mp4")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, rel), []byte("video-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	film := store.Client.Movie.Create().SetCode("ABP-001").SaveX(ctx)
	store.Client.File.Create().SetFileID("local-abc").SetName("ABP-001.mp4").SetSize(11).
		SetAccountID(domain.LocalAccountID).SetRootID(domain.LocalAccountID).SetPath(rel).SetMovieID(film.ID).ExecX(ctx)

	service := New(store.Client, nil, time.Minute, func(context.Context) ([]string, error) {
		return []string{root}, nil
	})

	pb, err := service.StartMovie(ctx, film.ID, "")
	if err != nil {
		t.Fatalf("start local playback: %v", err)
	}
	if pb.FileID != "local-abc" || len(pb.Sources) != 1 ||
		pb.Sources[0].Src != "/api/play-local/local-abc" || pb.Sources[0].Type != "video/mp4" ||
		pb.Source.AccountID != domain.LocalAccountID {
		t.Fatalf("unexpected local playback: %+v", pb)
	}

	abs, err := service.LocateLocalFile(ctx, "local-abc")
	if err != nil || abs != filepath.Join(root, rel) {
		t.Fatalf("locate local file = %q, %v", abs, err)
	}

	// MPV selection reports the local file so the link uses /api/play-local.
	pf, err := service.PrimaryFile(ctx, film.ID)
	if err != nil || pf.FileID != "local-abc" || !pf.Local || pf.Name != "ABP-001.mp4" {
		t.Fatalf("primary file = %+v, %v", pf, err)
	}

	// A record whose relative path escapes the configured root must be refused.
	store.Client.File.Create().SetFileID("local-out").SetName("escape.mp4").SetSize(1).
		SetAccountID(domain.LocalAccountID).SetRootID(domain.LocalAccountID).
		SetPath(filepath.Join("..", "escape.mp4")).SetMovieID(film.ID).ExecX(ctx)
	if _, err := service.LocateLocalFile(ctx, "local-out"); err == nil {
		t.Fatal("expected path traversal to be refused")
	}
}
