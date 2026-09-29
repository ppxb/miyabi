package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/config"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/emby"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestEmbyDirectoryAutomaticallyQueuesLocalScan(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	// TempDir may use a Windows short path; scan tasks store the resolved path.
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ABC-123.strm"), []byte("https://example.com/video"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{DataDir: t.TempDir(), EmbyDir: root, PublicURL: "http://localhost:8080", Listen: "127.0.0.1:0"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a, err := New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if a != nil {
			_ = a.Close()
		}
	}()
	record := a.store.Client.Task.Query().Where(task.TypeEQ("scan")).OnlyX(ctx)
	payload, err := tasks.DecodePayload[domain.ScanPayload](record.Payload)
	if err != nil || payload.Source.AccountID != "local" || payload.Source.Directory.Path != wantRoot {
		t.Fatalf("startup did not queue mounted directory: got path %q, want %q; payload %+v, error %v", payload.Source.Directory.Path, wantRoot, payload, err)
	}
	if count := a.store.Client.Movie.Query().CountX(ctx); count != 0 {
		t.Fatalf("startup imported synchronously: %d", count)
	}
	a.store.Client.Task.UpdateOneID(record.ID).SetStatus(task.StatusDone).ExecX(ctx)

	// Saving a new directory through the real settings handler queues that
	// directory, even though the startup environment still names the old one.
	newRoot := t.TempDir()
	wantNewRoot, err := filepath.EvalSymlinks(newRoot)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(emby.Config{LocalDir: newRoot, MediaPath: "/emby-container/media", PublicURL: cfg.PublicURL})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/settings/emby", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	a.server.Handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("save: %d %s", response.Code, response.Body.String())
	}
	queued := a.store.Client.Task.Query().Where(task.StatusEQ(task.StatusQueued)).OnlyX(ctx)
	payload, err = tasks.DecodePayload[domain.ScanPayload](queued.Payload)
	if err != nil || payload.Source.Directory.Path != wantNewRoot {
		t.Fatalf("save used stale directory: got path %q, want %q; error %v", payload.Source.Directory.Path, wantNewRoot, err)
	}

	response = httptest.NewRecorder()
	a.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/library/scan/local", nil))
	if response.Code != http.StatusAccepted {
		t.Fatalf("scan: %d %s", response.Code, response.Body.String())
	}
	var info domain.TaskInfo
	if err := json.Unmarshal(response.Body.Bytes(), &info); err != nil || info.ID != queued.ID {
		t.Fatalf("manual scan did not reuse active directory task: %+v %v", info, err)
	}

	// Restart restores the saved directory and reuses its pending scan.
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	a, err = New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	queued = a.store.Client.Task.Query().Where(task.StatusEQ(task.StatusQueued)).OnlyX(ctx)
	payload, err = tasks.DecodePayload[domain.ScanPayload](queued.Payload)
	if err != nil || payload.Source.Directory.Path != wantNewRoot {
		t.Fatalf("restart used stale directory: got path %q, want %q; error %v", payload.Source.Directory.Path, wantNewRoot, err)
	}
}
