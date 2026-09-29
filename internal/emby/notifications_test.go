package emby

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent"
)

// Exercise individual worker passes without a timer racing the test.
func notificationService(t *testing.T, db *ent.Client, cfg Config) *Service {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{db: db, cfg: cfg, client: newEmbyClient(), ctx: ctx, cancel: cancel, wakeNotifications: make(chan struct{}, 1)}
	s.actors = newActorSync(db, s.client, s.currentConfig, nil, nil)
	t.Cleanup(s.Close)
	return s
}

func TestNotificationsPersistFailureBackoffAndRecoverAfterRestart(t *testing.T) {
	var calls atomic.Int32
	var healthy atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Library/Media/Updated" {
			calls.Add(1)
			if !healthy.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	dir := t.TempDir()
	store, err := database.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Enabled: true, ServerURL: server.URL, APIKey: "key", LocalDir: t.TempDir()}
	s := notificationService(t, store.Client, cfg)
	for _, path := range []string{"A", "A", "B"} {
		if err := s.NotifyUpdated(t.Context(), filepath.Join(cfg.LocalDir, path)); err != nil {
			t.Fatal(err)
		}
	}
	if count := store.Client.EmbyNotification.Query().CountX(t.Context()); count != 2 {
		t.Fatalf("deduplicated count = %d", count)
	}
	s.flushNotifications(t.Context())
	for _, row := range store.Client.EmbyNotification.Query().AllX(t.Context()) {
		if row.Attempts != 1 || row.LastError == "" || time.Until(row.NextAttemptAt) <= 0 {
			t.Fatalf("missing failure/backoff: %+v", row)
		}
	}
	s.flushNotifications(t.Context())
	if calls.Load() != 1 {
		t.Fatal("retried before backoff elapsed")
	}
	s.Close()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = database.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if count := store.Client.EmbyNotification.Query().CountX(t.Context()); count != 2 {
		t.Fatalf("restart lost pending updates: %d", count)
	}
	resumed, err := NewService(t.Context(), store.Client, cfg, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	healthy.Store(true)
	if err := resumed.RetryPending(t.Context()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for store.Client.EmbyNotification.Query().CountX(t.Context()) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("manual retry did not bypass backoff")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() != 2 {
		t.Fatalf("requests = %d", calls.Load())
	}
	if got := notificationBackoff(100); got != 5*time.Minute {
		t.Fatalf("backoff is unbounded: %s", got)
	}
}

func TestNotificationTransactionsAndConcurrentUpdates(t *testing.T) {
	for _, manual := range []bool{false, true} {
		for _, success := range []bool{true, false} {
			t.Run(map[bool]string{true: "old success", false: "old failure"}[success]+map[bool]string{false: " new update", true: " manual retry"}[manual], func(t *testing.T) {
				started, release := make(chan struct{}), make(chan struct{})
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/Library/Media/Updated" && calls.Add(1) == 1 {
						close(started)
						<-release
						if !success {
							w.WriteHeader(http.StatusServiceUnavailable)
							return
						}
					}
					w.WriteHeader(http.StatusNoContent)
				}))
				defer server.Close()
				store, err := database.Open(t.Context(), t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = store.Close() })
				s := notificationService(t, store.Client, Config{Enabled: true, ServerURL: server.URL, APIKey: "key"})
				path := t.TempDir()
				tx, err := store.Client.Tx(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if err := s.NotifyUpdatedTx(t.Context(), tx, path); err != nil {
					t.Fatal(err)
				}
				if err := tx.Rollback(); err != nil {
					t.Fatal(err)
				}
				if store.Client.EmbyNotification.Query().CountX(t.Context()) != 0 {
					t.Fatal("rolled back scan left a notification")
				}
				if err := ent.WithTx(t.Context(), store.Client, func(tx *ent.Tx) error { return s.NotifyUpdatedTx(t.Context(), tx, path) }); err != nil {
					t.Fatal(err)
				}
				finished := make(chan struct{})
				go func() { defer close(finished); s.flushNotifications(t.Context()) }()
				<-started
				if manual {
					err = s.RetryPending(t.Context())
				} else {
					err = s.NotifyUpdated(t.Context(), path)
				}
				if err != nil {
					close(release)
					t.Fatal(err)
				}
				close(release)
				<-finished
				row := store.Client.EmbyNotification.Query().OnlyX(t.Context())
				if row.Revision != 2 || row.Attempts != 0 || row.LastError != "" {
					t.Fatalf("old request overwrote newer update: %+v", row)
				}
				s.flushNotifications(t.Context())
				if store.Client.EmbyNotification.Query().CountX(t.Context()) != 0 {
					t.Fatal("latest update was not delivered")
				}
			})
		}
	}
}
