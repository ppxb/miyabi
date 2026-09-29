package emby

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
)

type mediaFetcherFunc func(context.Context, string) (domain.Media, error)

func (f mediaFetcherFunc) Media(ctx context.Context, rawURL string) (domain.Media, error) {
	return f(ctx, rawURL)
}

func TestActorSync_FindAvatarFallsBackToJavDBMedia(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.Client.Actor.Create().SetJavdbID("actor-1").SetName("三上悠亜").SetNameZht("三上悠亞").
		SetAvatar("https://c0.jdbstatic.com/avatars/actor-1.jpg").ExecX(t.Context())

	svc := &actorSync{db: store.Client}
	want := domain.Media{ContentType: "image/png", Body: []byte("decoded")}
	media := mediaFetcherFunc(func(_ context.Context, rawURL string) (domain.Media, error) {
		if rawURL != "https://c0.jdbstatic.com/avatars/actor-1.jpg" {
			t.Errorf("fetched %q", rawURL)
		}
		return want, nil
	})
	got, found, err := svc.findAvatar(t.Context(), nil, media, "三上悠亞")
	if err != nil || !found || got.ContentType != want.ContentType || !bytes.Equal(got.Body, want.Body) {
		t.Fatalf("findAvatar = %+v, %v", got, found)
	}
	if _, found, err := svc.findAvatar(t.Context(), nil, media, "未知演员"); found || err != nil {
		t.Fatal("unknown actor produced an avatar")
	}
}

func TestActorSync_NegativeCache(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc := &actorSync{db: store.Client}
	actorName := "nonexistent-actor"

	// Initially not cached
	if svc.isAvatarNotFound(actorName) {
		t.Fatal("expected actor not in negative cache initially")
	}

	callCount := 0
	media := mediaFetcherFunc(func(_ context.Context, _ string) (domain.Media, error) {
		callCount++
		return domain.Media{}, nil
	})

	// First lookup: not found, marks negative cache
	_, found, err := svc.findAvatar(t.Context(), nil, media, actorName)
	if err != nil || found {
		t.Fatal("expected avatar not found")
	}
	svc.markAvatarNotFound(actorName)

	if !svc.isAvatarNotFound(actorName) {
		t.Fatal("expected actor to be in negative cache after marking")
	}

	// Now add actor to DB
	store.Client.Actor.Create().SetJavdbID("act-new").SetName(actorName).SetNameZht(actorName).
		SetAvatar("https://example.com/avatar.jpg").ExecX(t.Context())

	// Second lookup: hits negative cache, does not query DB or media fetcher
	_, found, err = svc.findAvatar(t.Context(), nil, media, actorName)
	if found {
		t.Fatal("expected negative cache to return false without looking up")
	}
	if callCount != 0 {
		t.Fatalf("expected 0 media calls due to negative cache, got %d", callCount)
	}

	// Clear negative cache
	svc.clearCache()
	if svc.isAvatarNotFound(actorName) {
		t.Fatal("expected negative cache cleared")
	}

	// Third lookup: cache cleared, finds actor
	_, found, err = svc.findAvatar(t.Context(), nil, media, actorName)
	if !found {
		t.Fatal("expected avatar found after clearing negative cache")
	}
	if callCount != 1 {
		t.Fatalf("expected 1 media call after cache cleared, got %d", callCount)
	}
}

func TestAvatarFailureIsRetriedOnNextSync(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.Client.Actor.Create().SetJavdbID("retry-actor").SetName("Retry Actor").SetAvatar("https://example.com/avatar.jpg").ExecX(t.Context())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/Persons" {
			_, _ = io.WriteString(w, `{"Items":[{"Id":"person-1","Name":"Retry Actor"}]}`)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	calls := 0
	media := mediaFetcherFunc(func(context.Context, string) (domain.Media, error) {
		calls++
		if calls == 1 {
			return domain.Media{}, errors.New("temporary network failure")
		}
		return domain.Media{ContentType: "image/png", Body: []byte("avatar")}, nil
	})
	svc, err := NewService(t.Context(), store.Client, Config{Enabled: true, ServerURL: server.URL, APIKey: "test"}, Dependencies{Media: media})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	if _, err := svc.actors.run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if svc.actors.isAvatarNotFound("Retry Actor") {
		t.Fatal("temporary failure cached as missing")
	}
	count, err := svc.actors.run(t.Context())
	if err != nil || count != 1 || calls != 2 {
		t.Fatalf("retry uploaded=%d calls=%d err=%v", count, calls, err)
	}
}
