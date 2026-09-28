package monitor

import (
	"context"
	"sync"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/subscription"
	"github.com/ppxb/miyabi/internal/magnet"
	"github.com/ppxb/miyabi/internal/tasks"
)

type mockDiscoverer struct {
	mu           sync.Mutex
	magnets      map[string][]domain.Magnet
	magnetsErr   error
	actorMovies  map[string][]domain.Movie
	browseErr    error
	summaryCalls int
}

func newMockDiscoverer() *mockDiscoverer {
	return &mockDiscoverer{magnets: map[string][]domain.Magnet{}, actorMovies: map[string][]domain.Movie{}}
}

func (m *mockDiscoverer) MovieSummary(_ context.Context, movieID string) (domain.MovieSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.summaryCalls++
	return domain.MovieSummary{ID: movieID, Code: "MOCK-" + movieID, Title: "Mock " + movieID, Cover: "https://example.com/cover.jpg", ReleaseDate: "2026-10-01"}, nil
}

func (m *mockDiscoverer) CatalogueMagnets(_ context.Context, movieID string) ([]domain.Magnet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.magnets[movieID], m.magnetsErr
}

func (m *mockDiscoverer) BrowseMovies(_ context.Context, options domain.BrowseOptions) ([]domain.Movie, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.browseErr != nil {
		return nil, m.browseErr
	}
	return m.actorMovies[options.EntityID], nil
}

type submission struct{ MovieID, Hash string }

type mockOfflineAdder struct {
	mu          sync.Mutex
	submissions []submission
}

func (m *mockOfflineAdder) Add(_ context.Context, movieID, hash string) (domain.OfflineSubmission, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.submissions = append(m.submissions, submission{movieID, hash})
	return domain.OfflineSubmission{TaskID: len(m.submissions), Code: movieID, JavDBID: movieID, Hash: hash, Status: "running"}, nil
}

type fixture struct {
	service  *Service
	discover *mockDiscoverer
	offline  *mockOfflineAdder
	tasks    *tasks.Service
	client   *ent.Client
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	discover, offline := newMockDiscoverer(), &mockOfflineAdder{}
	taskSvc := tasks.NewService(store.Client, tasks.NewRegistry())
	return fixture{New(store.Client, discover, offline, taskSvc), discover, offline, taskSvc, store.Client}
}

func TestMovieSubscriptionLifecycle(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	item, err := f.service.AddMovie(ctx, "m1", AddMovieOptions{})
	if err != nil || item.Code != "MOCK-m1" || item.Status != StatusWaiting || !item.AutoDownload {
		t.Fatalf("AddMovie: %#v %v", item, err)
	}
	if again, err := f.service.AddMovie(ctx, "m1", AddMovieOptions{}); err != nil || again.ID != item.ID || f.discover.summaryCalls != 1 {
		t.Fatalf("re-adding a waiting subscription must be a no-op without a catalogue lookup: %#v %v calls=%d", again, err, f.discover.summaryCalls)
	}
	if list, err := f.service.List(ctx, "movie", 1, 10); err != nil || len(list) != 1 {
		t.Fatalf("List: %d %v", len(list), err)
	}
	updated, err := f.service.Update(ctx, item.ID, UpdateOptions{AutoDownload: ptr(false), Zone: ptr("censored")})
	if err != nil || updated.AutoDownload || updated.Zone != "censored" {
		t.Fatalf("Update: %#v %v", updated, err)
	}
	if _, err := f.service.Update(ctx, item.ID, UpdateOptions{Status: ptr("paused")}); !domain.IsKind(err, domain.KindInvalid) {
		t.Fatalf("pausing a movie subscription must be rejected as invalid, got %v", err)
	}
	if err := f.service.Remove(ctx, item.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Remove(ctx, item.ID); !ent.IsNotFound(err) {
		t.Fatalf("removing a missing subscription must report not found, got %v", err)
	}
}

func TestReAddingFinishedSubscriptionOnlyResetsForUsers(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	item, _ := f.service.AddMovie(ctx, "m1", AddMovieOptions{})
	f.discover.magnets["m1"] = []domain.Magnet{{Hash: "h1", Name: "MOCK-m1", HasSubtitle: true}}
	if added, err := f.service.EnqueueSingle(ctx, item.ID); err != nil || added.Status != StatusAdded {
		t.Fatalf("EnqueueSingle: %#v %v", added, err)
	}
	spawned, err := f.service.addMovie(ctx, domain.MovieSummary{ID: "m1"}, AddMovieOptions{OriginID: ptr(99)})
	if err != nil || spawned.ID != item.ID || spawned.Status != StatusAdded {
		t.Fatalf("an actor-spawned add must leave an added subscription alone: %#v %v", spawned, err)
	}
	rearmed, err := f.service.AddMovie(ctx, "m1", AddMovieOptions{})
	if err != nil || rearmed.Status != StatusWaiting || rearmed.Hash != "" {
		t.Fatalf("a user re-add must re-arm: %#v %v", rearmed, err)
	}
	if len(f.offline.submissions) != 1 {
		t.Fatalf("expected exactly one 115 submission, got %d", len(f.offline.submissions))
	}
}

func TestSubscriptionSettings(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	cfg, err := f.service.Config(ctx)
	if err != nil || !cfg.MovieAutoDownload || cfg.ActorAutoDownload || cfg.CheckTime != "00:00" || cfg.Preferences != magnet.DefaultPreferences() {
		t.Fatalf("default config mismatch: %#v %v", cfg, err)
	}
	cfg.ActorAutoDownload, cfg.CheckTime, cfg.Preferences.Subtitle = true, "05:30", magnet.PreferenceRequired
	if err := f.service.UpdateConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if saved, err := f.service.Config(ctx); err != nil || saved != cfg {
		t.Fatalf("round trip mismatch: %#v %v", saved, err)
	}
	for name, mutate := range map[string]func(*Config){
		"hour out of range": func(c *Config) { c.CheckTime = "25:00" },
		"missing minutes":   func(c *Config) { c.CheckTime = "4" },
		"unknown level":     func(c *Config) { c.Preferences.HD = "sometimes" },
	} {
		bad := cfg
		mutate(&bad)
		if err := f.service.UpdateConfig(ctx, bad); !domain.IsKind(err, domain.KindInvalid) {
			t.Errorf("%s: expected KindInvalid, got %v", name, err)
		}
	}
	// A row written by an older build with blank fields normalizes on read.
	if err := database.SaveSetting(ctx, f.client, subscriptionConfigSetting, map[string]any{"movie_auto_download": false}); err != nil {
		t.Fatal(err)
	}
	legacy, err := f.service.Config(ctx)
	if err != nil || legacy.MovieAutoDownload || legacy.CheckTime != "00:00" || legacy.Preferences != magnet.DefaultPreferences() {
		t.Fatalf("legacy config must normalize: %#v %v", legacy, err)
	}
}

func TestSubscriptionTargetsAndPagination(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	f.discover.actorMovies["act1"] = []domain.Movie{
		{ID: "m-spawned", Code: "SPAWN-001", Title: "Spawned Movie", ReleaseDate: "2099-11-01", Actors: []domain.Actor{{ID: "act1", Name: "Actor 1"}}},
	}
	actor, err := f.service.AddActor(ctx, "act1", AddActorOptions{Title: "Actor 1"})
	if err != nil {
		t.Fatal(err)
	}
	m1, err := f.service.AddMovie(ctx, "m1", AddMovieOptions{})
	if err != nil {
		t.Fatal(err)
	}
	m2, err := f.service.AddMovie(ctx, "m2", AddMovieOptions{})
	if err != nil {
		t.Fatal(err)
	}

	targets, err := f.service.Targets(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 4 { // 1 actor + 2 movies + 1 spawned movie
		t.Fatalf("expected 4 targets, got %d", len(targets))
	}

	movieTargets, err := f.service.Targets(ctx, "movie")
	if err != nil {
		t.Fatal(err)
	}
	if len(movieTargets) != 3 {
		t.Fatalf("expected 3 movie targets, got %d", len(movieTargets))
	}

	// Pagination test
	page1, err := f.service.List(ctx, "movie", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 items on page 1, got %d", len(page1))
	}
	page2, err := f.service.List(ctx, "movie", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 1 {
		t.Fatalf("expected 1 item on page 2, got %d", len(page2))
	}

	// ActorFeed tests
	specificFeed, err := f.service.ActorFeed(ctx, actor.ID, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(specificFeed) != 1 || specificFeed[0].TargetID != "m-spawned" {
		t.Fatalf("expected 1 spawned movie for actor, got %#v", specificFeed)
	}

	allSpawnedFeed, err := f.service.ActorFeed(ctx, 0, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(allSpawnedFeed) != 1 || allSpawnedFeed[0].TargetID != "m-spawned" {
		t.Fatalf("expected 1 spawned movie in all spawned feed, got %#v", allSpawnedFeed)
	}

	_ = m1
	_ = m2
}

func TestMovieSubscription_ConstraintErrorRecoversWithoutRecursion(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	var wg sync.WaitGroup
	n := 10
	results := make([]Item, n)
	errors := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errors[idx] = f.service.addMovie(ctx, domain.MovieSummary{
				ID: "m-race", Code: "MOCK-race", Title: "Race Title",
			}, AddMovieOptions{})
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errors[i] != nil {
			t.Fatalf("expected concurrent addMovie to succeed via constraint recovery, got error at %d: %v", i, errors[i])
		}
		if results[i].ID != results[0].ID {
			t.Fatalf("expected all results to have the same ID, got %d vs %d", results[i].ID, results[0].ID)
		}
	}

	count, err := f.client.Subscription.Query().Where(subscription.KindEQ(subscription.KindMovie), subscription.TargetIDEQ("m-race")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 subscription created, got %d", count)
	}
}

func TestActorSubscription_ConstraintErrorRecoversWithoutRecursion(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	var wg sync.WaitGroup
	n := 10
	results := make([]Item, n)
	errors := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errors[idx] = f.service.AddActor(ctx, "act-race", AddActorOptions{
				Title: "Actor Race",
			})
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errors[i] != nil {
			t.Fatalf("expected concurrent AddActor to succeed via constraint recovery, got error at %d: %v", i, errors[i])
		}
		if results[i].ID != results[0].ID {
			t.Fatalf("expected all results to have the same ID, got %d vs %d", results[i].ID, results[0].ID)
		}
	}

	count, err := f.client.Subscription.Query().Where(subscription.KindEQ(subscription.KindActor), subscription.TargetIDEQ("act-race")).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 subscription created, got %d", count)
	}
}

