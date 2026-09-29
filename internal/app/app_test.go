package app_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/app"
	"github.com/ppxb/miyabi/internal/config"
)

func TestAppLifecycle(t *testing.T) {
	cfg := &config.Config{
		DataDir:  t.TempDir(),
		Listen:   "127.0.0.1:0",
		LogLevel: "info",
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

func TestCheckHealth(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/health" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		_, port, err := net.SplitHostPort(server.Listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		if err := app.CheckHealth("127.0.0.1:" + port); err != nil {
			t.Fatalf("expected healthcheck success, got: %v", err)
		}
	})

	t.Run("unhealthy status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		if err := app.CheckHealth(server.Listener.Addr().String()); err == nil {
			t.Fatal("expected healthcheck failure for status 503")
		}
	})

	t.Run("invalid address", func(t *testing.T) {
		if err := app.CheckHealth("invalid-listen-address"); err == nil {
			t.Fatal("expected error for invalid address")
		}
	})
}
