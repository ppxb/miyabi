package setup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
)

func TestServiceLifecycle(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	service, err := New(ctx, store.Client)
	if err != nil {
		t.Fatal(err)
	}
	if !service.NeedsSetup() {
		t.Fatal("fresh service must need setup")
	}
	if err := service.Complete(ctx, []string{"unknown"}); err == nil {
		t.Fatal("expected unsupported source error")
	}
	if err := service.Complete(ctx, nil); err == nil {
		t.Fatal("expected empty source error")
	}

	media := filepath.Join(t.TempDir(), "media")
	if err := os.Mkdir(media, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := service.SetLocalDirectory(ctx, media); err != nil {
		t.Fatal(err)
	}
	if got, err := service.LocalDirectory(ctx); err != nil || got == "" {
		t.Fatalf("local directory = %q, error = %v", got, err)
	}
	if err := service.SetLocalDirectory(ctx, filepath.Join(media, "missing")); err == nil {
		t.Fatal("expected missing directory error")
	}

	if err := service.Complete(ctx, []string{"115", "local"}); err != nil {
		t.Fatal(err)
	}
	state := service.Status()
	if !state.Completed || len(state.Sources) != 2 || state.CompletedAt == nil {
		t.Fatalf("unexpected state: %#v", state)
	}

	reloaded, err := New(ctx, store.Client)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.NeedsSetup() || len(reloaded.Status().Sources) != 2 {
		t.Fatalf("state not persisted: %#v", reloaded.Status())
	}
}
