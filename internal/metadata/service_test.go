package metadata

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
)

type sourceStub struct {
	id    string
	calls atomic.Int32
	fetch func(context.Context, string) (domain.MovieMetadata, error)
}

func (s *sourceStub) ID() string         { return s.id }
func (*sourceStub) Supports(string) bool { return true }
func (s *sourceStub) Fetch(ctx context.Context, code string) (domain.MovieMetadata, error) {
	s.calls.Add(1)
	return s.fetch(ctx, code)
}
func (*sourceStub) Media(context.Context, string) (domain.Media, error) { return domain.Media{}, nil }
func fixture(provider, code, title string) domain.MovieMetadata {
	return domain.MovieMetadata{Detail: domain.MovieDetail{Movie: domain.Movie{Code: code, Title: title, Sources: []domain.SourceID{{Provider: provider, ID: code}}}}}
}
func newTestService(t *testing.T, sources ...Source) *Service {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s, err := New(t.Context(), store.Client, sources...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestResolveWithoutJavDBMergesInPriorityOrderAndPersistsCache(t *testing.T) {
	primary := &sourceStub{id: "primary", fetch: func(_ context.Context, code string) (domain.MovieMetadata, error) {
		m := fixture("primary", code, "Primary title")
		m.Detail.Actors = []domain.Actor{{Provider: "primary", ID: "a", Name: "Same name"}}
		return m, nil
	}}
	secondary := &sourceStub{id: "secondary", fetch: func(_ context.Context, code string) (domain.MovieMetadata, error) {
		m := fixture("secondary", code, "Secondary title")
		m.Detail.Summary = "Synopsis"
		m.Detail.Rating = 8
		m.Detail.RatingMax = 10
		m.Detail.Actors = []domain.Actor{{Provider: "secondary", ID: "a", Name: "Same name"}}
		m.Images = []domain.ImageCandidate{{Provider: "secondary", URL: "https://image.example/full.jpg", Role: "cover"}}
		return m, nil
	}}
	s := newTestService(t, primary, secondary)
	for range 2 {
		m, err := s.Resolve(t.Context(), domain.MovieRef{Code: "ABP-123"})
		if err != nil {
			t.Fatal(err)
		}
		if m.Detail.ID != "" || m.Detail.Title != "Primary title" || m.Detail.Summary != "Synopsis" || len(m.Detail.Actors) != 1 || m.Detail.Actors[0].Provider != "primary" || m.Detail.RatingSource != "secondary" || m.Detail.RatingMax != 10 || len(m.Images) != 1 {
			t.Fatalf("unexpected merge: %+v", m)
		}
	}
	if primary.calls.Load() != 1 || secondary.calls.Load() != 1 {
		t.Fatal("hot cache fetched sources again")
	}
	if err := s.UpdateSettings(t.Context(), []SourceSetting{{ID: "secondary", Enabled: true}, {ID: "primary", Enabled: false}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Resolve(t.Context(), domain.MovieRef{Code: "ABP-123"})
	if err != nil || got.Detail.Title != "Secondary title" {
		t.Fatalf("disabled source contributed: %+v %v", got, err)
	}
	if primary.calls.Load() != 1 || secondary.calls.Load() != 1 {
		t.Fatal("settings change discarded source cache")
	}
	// A separate service instance reads the persisted source results and settings.
	reopened, err := New(t.Context(), s.db, primary, secondary)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.Resolve(t.Context(), domain.MovieRef{Code: "ABP-123"}); err != nil {
		t.Fatal(err)
	}
	if secondary.calls.Load() != 1 {
		t.Fatal("cache did not survive service recreation")
	}
}

func TestConcurrentQueriesShareOneFetchAndCallerCancellation(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	source := &sourceStub{id: "fixture", fetch: func(ctx context.Context, code string) (domain.MovieMetadata, error) {
		close(started)
		select {
		case <-release:
			return fixture("fixture", code, "Title"), nil
		case <-ctx.Done():
			return domain.MovieMetadata{}, ctx.Err()
		}
	}}
	s := newTestService(t, source)
	ctx, cancel := context.WithCancel(t.Context())
	first := make(chan error, 1)
	go func() { _, err := s.Resolve(ctx, domain.MovieRef{Code: "ABP-123"}); first <- err }()
	<-started
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			if _, err := s.Resolve(t.Context(), domain.MovieRef{Code: "ABP-123"}); err != nil {
				t.Error(err)
			}
		})
	}
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	close(release)
	wg.Wait()
	if source.calls.Load() != 1 {
		t.Fatalf("concurrent callers fetched %d times", source.calls.Load())
	}
}

func TestSourceFailureIsRetriedAndCannotDowngradeIdentity(t *testing.T) {
	source := &sourceStub{id: "fixture"}
	source.fetch = func(_ context.Context, code string) (domain.MovieMetadata, error) {
		if source.calls.Load() == 1 {
			return domain.MovieMetadata{}, errors.New("network unavailable")
		}
		return fixture("fixture", code, "Recovered"), nil
	}
	s := newTestService(t, source)
	if _, err := s.Resolve(t.Context(), domain.MovieRef{Code: "118ABP-123"}); err == nil {
		t.Fatal("network failure was ignored")
	}
	if source.calls.Load() != 1 {
		t.Fatal("network failure triggered weaker matching")
	}
	if _, err := s.Resolve(t.Context(), domain.MovieRef{Code: "118ABP-123"}); err != nil {
		t.Fatal(err)
	}
	if source.calls.Load() != 2 {
		t.Fatal("network error was cached as not found")
	}
}

func TestWrongFilmIsNeverCachedOrMerged(t *testing.T) {
	wrong := &sourceStub{id: "wrong", fetch: func(context.Context, string) (domain.MovieMetadata, error) {
		return fixture("wrong", "ABP-124", "Wrong"), nil
	}}
	s := newTestService(t, wrong)
	if _, err := s.Resolve(t.Context(), domain.MovieRef{Code: "ABP-123"}); err == nil {
		t.Fatal("accepted wrong film")
	}
	if got := s.db.MetadataCache.Query().CountX(t.Context()); got != 0 {
		t.Fatalf("cached %d invalid matches", got)
	}
}

func TestConfirmedMissIsCachedAndSourceSettingsValidate(t *testing.T) {
	source := &sourceStub{id: "fixture", fetch: func(context.Context, string) (domain.MovieMetadata, error) {
		return domain.MovieMetadata{}, ErrNotFound
	}}
	s := newTestService(t, source)
	for range 2 {
		if _, err := s.Resolve(t.Context(), domain.MovieRef{Code: "ABP-123"}); !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
	}
	if source.calls.Load() != 1 {
		t.Fatalf("negative cache missed: %d", source.calls.Load())
	}
	if err := s.UpdateSettings(t.Context(), []SourceSetting{{ID: "unknown", Enabled: true}}); err == nil {
		t.Fatal("unknown source accepted")
	}
}
