package emby

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/export"
)

func TestConstructorRestoresExportConfigAndInjectsScanScheduler(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	saved := Config{LocalDir: t.TempDir(), PublicURL: "http://saved.example"}
	if err := database.SaveSetting(ctx, store.Client, SettingKey, saved); err != nil {
		t.Fatal(err)
	}
	mgr := export.NewManager(export.Config{EmbyDir: t.TempDir(), PublicURL: "http://default.example", STRMToken: "playback-token"})
	var scheduled []export.Config
	svc, err := NewService(ctx, store.Client, Config{LocalDir: mgr.Config().EmbyDir, PublicURL: mgr.Config().PublicURL}, Dependencies{
		ExportManager:     mgr,
		ScheduleLocalScan: func(context.Context) error { scheduled = append(scheduled, mgr.Config()); return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	want := export.Config{EmbyDir: saved.LocalDir, PublicURL: saved.PublicURL, STRMToken: "playback-token"}
	if got := mgr.Config(); got != want {
		t.Fatalf("constructor returned with stale export config: %+v", got)
	}
	want.EmbyDir = t.TempDir()
	if err := svc.UpdateConfig(ctx, Config{LocalDir: want.EmbyDir, PublicURL: want.PublicURL}); err != nil {
		t.Fatal(err)
	}
	if len(scheduled) != 1 || scheduled[0] != want {
		t.Fatalf("scheduler saw incomplete dependencies or stale configuration: %+v", scheduled)
	}
}

func TestSwitchLocalDirectoryRewritesSTRMWithoutChangingPublicURL(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	oldRoot, newRoot := t.TempDir(), t.TempDir()
	mgr := export.NewManager(export.Config{EmbyDir: oldRoot, PublicURL: "http://current.example", STRMToken: "current-token"})
	svc, err := NewService(ctx, store.Client, Config{LocalDir: oldRoot, PublicURL: mgr.Config().PublicURL}, Dependencies{ExportManager: mgr})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	stale := export.STRMContent("http://previous.example", "123", "old-token")
	for _, root := range []string{oldRoot, newRoot} {
		if err := os.WriteFile(filepath.Join(root, "ABC-123.strm"), stale, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.UpdateConfig(ctx, Config{LocalDir: newRoot, PublicURL: mgr.Config().PublicURL}); err != nil {
		t.Fatal(err)
	}
	want := export.STRMContent("http://current.example", "123", "current-token")
	deadline := time.Now().Add(3 * time.Second)
	for {
		got, err := os.ReadFile(filepath.Join(newRoot, "ABC-123.strm"))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(got, want) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("new directory retained stale STRM: %s", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
	got, err := os.ReadFile(filepath.Join(oldRoot, "ABC-123.strm"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, stale) {
		t.Fatalf("previous directory was unexpectedly rewritten: %s", got)
	}
}

func TestEmbyConfig_Normalize(t *testing.T) {
	cfg := Config{
		Enabled:   true,
		ServerURL: "  http://192.168.1.100:8096/  ",
		APIKey:    "  secret-key  ",
		MediaPath: "  /media  ",
		LocalDir:  "  /data/emby  ",
	}

	if err := cfg.Normalize(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ServerURL != "http://192.168.1.100:8096" {
		t.Errorf("expected trimmed server URL, got %q", cfg.ServerURL)
	}
	if cfg.APIKey != "secret-key" {
		t.Errorf("expected trimmed APIKey, got %q", cfg.APIKey)
	}
	if cfg.MediaPath != "/media" {
		t.Errorf("expected trimmed MediaPath, got %q", cfg.MediaPath)
	}
	if cfg.LocalDir != "/data/emby" {
		t.Errorf("expected trimmed LocalDir, got %q", cfg.LocalDir)
	}

	// Missing server URL when enabled
	invalid := Config{Enabled: true, APIKey: "key"}
	if err := invalid.Normalize(); err == nil {
		t.Error("expected error for missing server URL")
	}

	// Missing API key when enabled
	invalid = Config{Enabled: true, ServerURL: "http://192.168.1.100:8096"}
	if err := invalid.Normalize(); err == nil {
		t.Error("expected error for missing API key")
	}

	// Disabled does not require URL or key
	disabled := Config{Enabled: false}
	if err := disabled.Normalize(); err != nil {
		t.Errorf("expected no error for disabled config, got %v", err)
	}
}

func TestEmbyService_TranslatePath(t *testing.T) {
	// Standard relative inside localDir
	got := translatePath("/app/data/emby/IPX/IPX-123", "/app/data/emby", "/media")
	if got != "/media/IPX/IPX-123" {
		t.Errorf("expected /media/IPX/IPX-123, got %q", got)
	}

	// Subdirectory starting with .. inside localDir (should not be treated as escaping)
	got = translatePath("/app/data/emby/..foo/IPX-123", "/app/data/emby", "/media")
	if got != "/media/..foo/IPX-123" {
		t.Errorf("expected /media/..foo/IPX-123, got %q", got)
	}

	// Empty media path returns local path with forward slashes
	got = translatePath("/app/data/emby/IPX/IPX-123", "/app/data/emby", "")
	if got != "/app/data/emby/IPX/IPX-123" {
		t.Errorf("expected /app/data/emby/IPX/IPX-123, got %q", got)
	}

	// Relative path with empty media path resolves to absolute
	got = translatePath("data/emby/IPX/IPX-123", "data/emby", "")
	if !filepath.IsAbs(got) {
		t.Errorf("expected absolute path for relative input with empty media path, got %q", got)
	}

	// Path outside localDir returns empty string (should be skipped)
	got = translatePath("/other/folder/movie", "/app/data/emby", "/media")
	if got != "" {
		t.Errorf("expected empty string for path outside localDir, got %q", got)
	}

	// Escaping path via .. returns empty string
	got = translatePath("/app/data/emby/../other/movie", "/app/data/emby", "/media")
	if got != "" {
		t.Errorf("expected empty string for escaping path, got %q", got)
	}
}

func TestEmbyService_Ping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/System/Info" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Emby-Token") != "valid-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"ServerName": "TestServer",
			"Version":    "4.8.8.0",
			"Id":         "srv-1",
		})
	}))
	defer server.Close()

	store, err := database.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("database.Open failed: %v", err)
	}
	defer store.Close()

	svc, err := NewService(context.Background(), store.Client, Config{
		Enabled:   true,
		ServerURL: server.URL,
		APIKey:    "valid-token",
	}, Dependencies{})
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	defer svc.Close()

	info, err := svc.Test(context.Background(), Config{ServerURL: server.URL, APIKey: "valid-token"})
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	if info.ServerName != "TestServer" || info.Version != "4.8.8.0" {
		t.Errorf("unexpected info: %+v", info)
	}

	// Invalid token
	_, err = svc.Test(context.Background(), Config{ServerURL: server.URL, APIKey: "wrong-token"})
	if err == nil {
		t.Error("expected unauthorized error for invalid token")
	}
}

func TestEmbyService_NotifyUpdatedBatch(t *testing.T) {
	receivedUpdates := make(chan []mediaUpdateItem, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Library/Media/Updated" && r.Method == http.MethodPost {
			var req mediaUpdateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				receivedUpdates <- req.Updates
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/Library/Refresh" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	store, err := database.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("database.Open failed: %v", err)
	}
	defer store.Close()

	svc, err := NewService(context.Background(), store.Client, Config{
		Enabled:   true,
		ServerURL: server.URL,
		APIKey:    "valid-token",
		LocalDir:  "/app/data/emby",
		MediaPath: "/media",
	}, Dependencies{})
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	defer svc.Close()

	// Enqueue notifications (with duplicates)
	svc.NotifyUpdated("/app/data/emby/IPX/IPX-123")
	svc.NotifyUpdated("/app/data/emby/IPX/IPX-123")
	svc.NotifyUpdated("/app/data/emby/SSIS/SSIS-456")

	select {
	case updates := <-receivedUpdates:
		if len(updates) != 2 {
			t.Fatalf("expected 2 deduplicated updates, got %d: %+v", len(updates), updates)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for batched updates")
	}
}

func TestEmbyService_NotifyUpdatedDeleted(t *testing.T) {
	receivedUpdates := make(chan []mediaUpdateItem, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Library/Media/Updated" && r.Method == http.MethodPost {
			var req mediaUpdateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				receivedUpdates <- req.Updates
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/Library/Refresh" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	store, err := database.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("database.Open failed: %v", err)
	}
	defer store.Close()

	tempDir := t.TempDir()
	localDir := filepath.Join(tempDir, "emby")
	existingMovieDir := filepath.Join(localDir, "IPX", "IPX-123")
	if err := os.MkdirAll(existingMovieDir, 0o755); err != nil {
		t.Fatal(err)
	}
	deletedMovieDir := filepath.Join(localDir, "SSIS", "SSIS-456")

	svc, err := NewService(context.Background(), store.Client, Config{
		Enabled:   true,
		ServerURL: server.URL,
		APIKey:    "valid-token",
		LocalDir:  localDir,
		MediaPath: "/media",
	}, Dependencies{})
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	defer svc.Close()

	svc.NotifyUpdated(existingMovieDir)
	svc.NotifyUpdated(deletedMovieDir)

	select {
	case updates := <-receivedUpdates:
		if len(updates) != 2 {
			t.Fatalf("expected 2 updates, got %d: %+v", len(updates), updates)
		}
		updateMap := make(map[string]string)
		for _, u := range updates {
			updateMap[u.Path] = u.UpdateType
		}
		if updateMap["/media/IPX/IPX-123"] != "Created" {
			t.Errorf("expected Created for existing movie, got %s", updateMap["/media/IPX/IPX-123"])
		}
		if updateMap["/media/SSIS/SSIS-456"] != "Deleted" {
			t.Errorf("expected Deleted for missing movie, got %s", updateMap["/media/SSIS/SSIS-456"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for batched updates")
	}
}
