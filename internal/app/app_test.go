package app_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
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

func TestLocalURLUsesLoopbackForWildcardListeners(t *testing.T) {
	for _, test := range []struct{ listen, want string }{
		{":8080", "http://127.0.0.1:8080"},
		{"0.0.0.0:8080", "http://127.0.0.1:8080"},
		{"127.0.0.1:9090", "http://127.0.0.1:9090"},
		{"[::]:8080", "http://[::1]:8080"},
		{"[::1]:8080", "http://[::1]:8080"},
		{"192.168.1.5:8080", "http://192.168.1.5:8080"},
	} {
		t.Run(test.listen, func(t *testing.T) {
			got, err := app.LocalURL(test.listen)
			if err != nil || got != test.want {
				t.Fatalf("LocalURL(%q) = %q, %v; want %q", test.listen, got, err, test.want)
			}
		})
	}
	if _, err := app.LocalURL("invalid-listen-address"); err == nil {
		t.Fatal("expected an error for an invalid listen address")
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
