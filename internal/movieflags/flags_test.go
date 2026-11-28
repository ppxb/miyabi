package movieflags

import (
	"context"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
)

func TestLoadDerivesSourceAndFileFlags(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	film := store.Client.Movie.Create().SetCode("ABP-001").SaveX(ctx)
	store.Client.File.Create().SetFileID("f1").SetName("ABP-001-UC.mp4").SetSize(1).
		SetAccountID("account").SetRootID("root").SetMovieID(film.ID).ExecX(ctx)
	store.Client.File.Create().SetFileID("f2").SetName("ABP-001.chs.srt").SetSize(1).
		SetMovieID(film.ID).ExecX(ctx)
	store.Client.Subtitle.Create().SetMovieID(film.ID).SetName("ABP-001.chs.srt").ExecX(ctx)

	providers := map[string]string{"account": "guangya"}
	mounted := map[[2]string]bool{{"account", "root"}: true}
	flags, err := Load(ctx, store.Client, []int{film.ID}, providers, mounted)
	if err != nil {
		t.Fatal(err)
	}
	if flags.Source[film.ID] != "guangya" || !flags.HasSubtitle[film.ID] || !flags.HasUncensored[film.ID] {
		t.Fatalf("flags = %+v", flags)
	}
	if flags.HasCracked[film.ID] {
		t.Fatal("unexpected cracked flag")
	}

	flags, err = Load(ctx, store.Client, []int{film.ID}, providers, nil)
	if err != nil || flags.Source[film.ID] != "" {
		t.Fatalf("unmounted source = %+v, %v", flags.Source, err)
	}
}
