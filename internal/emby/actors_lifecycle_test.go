package emby

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
)

func TestActorCloseCancelsAndWaitsForScheduledRun(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	cfg := Config{Enabled: true, ServerURL: server.URL, APIKey: "key"}
	actors := newActorSync(nil, newEmbyClient(), func() Config { return cfg }, nil, nil)
	defer actors.close()
	actors.schedule(0)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("scheduled run did not start")
	}
	if _, err := actors.run(t.Context()); !errors.Is(err, errActorSyncBusy) {
		t.Fatalf("concurrent run: %v", err)
	}
	closed := make(chan struct{})
	go func() { actors.close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("close did not cancel running request")
	}
	actors.mu.Lock()
	running := actors.running
	actors.mu.Unlock()
	if running {
		t.Fatal("close returned before run completed")
	}
	actors.schedule(0)
	if _, err := actors.run(t.Context()); !errors.Is(err, context.Canceled) {
		t.Fatalf("closed sync restarted: %v", err)
	}
}

func TestConfigChangeCancelsOldAvatarUploadAndNextRunUsesNewServer(t *testing.T) {
	var oldUploads, newUploads atomic.Int32
	serve := func(id string, uploads *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/Persons":
				io.WriteString(w, `{"Items":[{"Id":"`+id+`","Name":"Actor"}]}`)
			case "/Items/" + id + "/Images/Primary":
				uploads.Add(1)
				w.WriteHeader(http.StatusNoContent)
			default:
				t.Errorf("unexpected server/person combination: %s", r.URL.Path)
				http.NotFound(w, r)
			}
		}))
	}
	oldServer, newServer := serve("old", &oldUploads), serve("new", &newUploads)
	defer oldServer.Close()
	defer newServer.Close()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.Client.Actor.Create().SetJavdbID("actor").SetName("Actor").SetAvatar("https://image.example/actor").ExecX(t.Context())
	entered := make(chan struct{})
	var calls atomic.Int32
	media := mediaFetcherFunc(func(ctx context.Context, _ string) (domain.Media, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-ctx.Done()
			return domain.Media{}, ctx.Err()
		}
		return domain.Media{ContentType: "image/jpeg", Body: []byte("image")}, nil
	})
	svc, err := NewService(t.Context(), store.Client, Config{Enabled: true, ServerURL: oldServer.URL, APIKey: "key"}, Dependencies{Media: media})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	finished := make(chan error, 1)
	go func() { _, err := svc.actors.run(t.Context()); finished <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("avatar lookup did not start")
	}
	if err := svc.UpdateConfig(t.Context(), Config{Enabled: true, ServerURL: newServer.URL, APIKey: "new-key"}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("old run: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("old run was not canceled")
	}
	count, err := svc.actors.run(t.Context())
	if err != nil || count != 1 || oldUploads.Load() != 0 || newUploads.Load() != 1 {
		t.Fatalf("count=%d old=%d new=%d err=%v", count, oldUploads.Load(), newUploads.Load(), err)
	}
}

func TestServiceCloseFlushesBufferedNotifications(t *testing.T) {
	updates := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Library/Media/Updated" {
			updates <- struct{}{}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc, err := NewService(t.Context(), store.Client, Config{Enabled: true, ServerURL: server.URL, APIKey: "key"}, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	svc.NotifyUpdated(t.TempDir())
	svc.Close()
	select {
	case <-updates:
	default:
		t.Fatal("buffered update was lost on close")
	}
}
