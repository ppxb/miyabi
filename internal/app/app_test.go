package app_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/app"
	"github.com/ppxb/miyabi/internal/config"
)

func TestAppLifecycle(t *testing.T) {
	dataDir := t.TempDir()
	cfg := &config.Config{
		DataDir:  dataDir,
		EmbyDir:  filepath.Join(dataDir, "emby"),
		Listen:   "127.0.0.1:0",
		LogLevel: slog.LevelInfo,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	application, err := app.New(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() {
		runErr <- application.Run(ctx)
	}()

	// Cancel to initiate graceful shutdown.
	cancel()

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("unexpected error during app run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for app shutdown")
	}

	if err := application.Close(); err != nil {
		t.Fatalf("failed to close app: %v", err)
	}
}
