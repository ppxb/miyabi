package emby

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/gfriends"
)

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
	s := &Service{}

	// Standard relative inside localDir
	got := s.translatePath("/app/data/emby/IPX/IPX-123", "/app/data/emby", "/media")
	if got != "/media/IPX/IPX-123" {
		t.Errorf("expected /media/IPX/IPX-123, got %q", got)
	}

	// Empty media path returns local path with forward slashes
	got = s.translatePath("/app/data/emby/IPX/IPX-123", "/app/data/emby", "")
	if got != "/app/data/emby/IPX/IPX-123" {
		t.Errorf("expected /app/data/emby/IPX/IPX-123, got %q", got)
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
	})
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	defer svc.Close()

	info, err := svc.Ping(context.Background(), server.URL, "valid-token")
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	if info.ServerName != "TestServer" || info.Version != "4.8.8.0" {
		t.Errorf("unexpected info: %+v", info)
	}

	// Invalid token
	_, err = svc.Ping(context.Background(), server.URL, "wrong-token")
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
	})
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

func TestEmbyService_SyncActorAvatars(t *testing.T) {
	uploadedAvatars := make(chan string, 1)
	gfriendsImage := []byte("gfriends-jpeg-data")

	embyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Persons" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Items": []map[string]any{
					{
						"Id":   "person-1",
						"Name": "三上悠亜",
						// missing avatar
					},
					{
						"Id":              "person-2",
						"Name":            "相沢みなみ",
						"PrimaryImageTag": "has-tag", // already has avatar
					},
				},
				"TotalRecordCount": 2,
			})
			return
		}
		if r.URL.Path == "/Items/person-1/Images/Primary" && r.Method == http.MethodPost {
			if r.Header.Get("Content-Type") != "image/jpeg" {
				t.Errorf("expected image/jpeg content type, got %s", r.Header.Get("Content-Type"))
			}
			body, _ := io.ReadAll(r.Body)
			decoded, err := base64.StdEncoding.DecodeString(string(body))
			if err != nil {
				t.Errorf("expected valid base64 payload: %v", err)
			}
			if !bytes.Equal(decoded, gfriendsImage) {
				t.Errorf("expected decoded image bytes %q, got %q", gfriendsImage, decoded)
			}
			uploadedAvatars <- "person-1"
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer embyServer.Close()

	// GFriends mock server
	tree := map[string]any{
		"Content": map[string]any{
			"S": map[string]string{
				"三上悠亜.jpg": "三上悠亜.jpg?t=1",
			},
		},
	}
	treeBytes, _ := json.Marshal(tree)

	gfriendsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Filetree.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(treeBytes)
			return
		}
		if r.URL.Path == "/Content/S/三上悠亜.jpg" {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(gfriendsImage)
			return
		}
		http.NotFound(w, r)
	}))
	defer gfriendsServer.Close()

	store, err := database.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("database.Open failed: %v", err)
	}
	defer store.Close()

	syncActors := true
	svc, err := NewService(context.Background(), store.Client, Config{
		Enabled:    true,
		ServerURL:  embyServer.URL,
		APIKey:     "valid-token",
		SyncActors: &syncActors,
	})
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	defer svc.Close()

	// Configure gfriends client with custom fastly URL via test cache
	cacheDir := t.TempDir()
	_ = os.WriteFile(cacheDir+"/gfriends_tree.json", treeBytes, 0644)
	gClient := gfriends.New(cacheDir, gfriendsServer.Client())
	svc.SetGFriends(gClient)

	// Test ListPersonsWithoutAvatar
	missing, err := svc.ListPersonsWithoutAvatar(t.Context())
	if err != nil {
		t.Fatalf("ListPersonsWithoutAvatar failed: %v", err)
	}
	if len(missing) != 1 || missing[0].Name != "三上悠亜" {
		t.Fatalf("expected 1 missing person '三上悠亜', got: %+v", missing)
	}

	// Test UploadPersonAvatar directly
	if err := svc.UploadPersonAvatar(t.Context(), "person-1", domain.Media{ContentType: "image/jpeg", Body: gfriendsImage}); err != nil {
		t.Fatalf("UploadPersonAvatar failed: %v", err)
	}

	select {
	case id := <-uploadedAvatars:
		if id != "person-1" {
			t.Errorf("expected upload for person-1, got %s", id)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for avatar upload")
	}
}

type mediaFetcherFunc func(context.Context, string) (domain.Media, error)

func (f mediaFetcherFunc) Media(ctx context.Context, rawURL string) (domain.Media, error) {
	return f(ctx, rawURL)
}

func TestEmbyService_FindAvatarFallsBackToJavDBMedia(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.Client.Actor.Create().SetJavdbID("actor-1").SetName("三上悠亜").SetNameZht("三上悠亞").
		SetAvatar("https://c0.jdbstatic.com/avatars/actor-1.jpg").ExecX(t.Context())

	svc := &Service{db: store.Client}
	want := domain.Media{ContentType: "image/png", Body: []byte("decoded")}
	media := mediaFetcherFunc(func(_ context.Context, rawURL string) (domain.Media, error) {
		if rawURL != "https://c0.jdbstatic.com/avatars/actor-1.jpg" {
			t.Errorf("fetched %q", rawURL)
		}
		return want, nil
	})
	got, found := svc.findAvatar(t.Context(), nil, media, "三上悠亞")
	if !found || got.ContentType != want.ContentType || !bytes.Equal(got.Body, want.Body) {
		t.Fatalf("findAvatar = %+v, %v", got, found)
	}
	if _, found := svc.findAvatar(t.Context(), nil, media, "未知演员"); found {
		t.Fatal("unknown actor produced an avatar")
	}
}
