package metadata

import (
	"context"
	"errors"
	"reflect"
	"strings"
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
func (s *sourceStub) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	s.calls.Add(1)
	return s.fetch(ctx, ref.Code)
}
func (*sourceStub) Media(context.Context, string) (domain.Media, error) { return domain.Media{}, nil }
func fixture(provider, code, title string) domain.MovieMetadata {
	return domain.MovieMetadata{Detail: domain.MovieDetail{Movie: domain.Movie{Code: code, Title: title, Sources: []domain.SourceID{{Provider: provider, ID: code}}}}}
}

func TestRefreshBypassesSuccessfulAndMissingSourceCaches(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "miss"}[missing], func(t *testing.T) {
			fresh := false
			source := &sourceStub{id: "official", fetch: func(_ context.Context, code string) (domain.MovieMetadata, error) {
				if missing && !fresh {
					return domain.MovieMetadata{}, ErrNotFound
				}
				title := "Old title"
				if fresh {
					title = "New title"
				}
				return fixture("official", code, title), nil
			}}
			s := newTestService(t, source)
			_, _ = s.Resolve(t.Context(), domain.MovieRef{Code: "EBWH-367"})
			fresh = true
			m, err := s.Resolve(t.Context(), domain.MovieRef{Code: "EBWH-367", Refresh: true})
			if err != nil || m.Detail.Title != "New title" || source.calls.Load() != 2 {
				t.Fatalf("refresh used cached metadata: %+v %v", m, err)
			}
			m, err = s.Resolve(t.Context(), domain.MovieRef{Code: "EBWH-367"})
			if err != nil || m.Detail.Title != "New title" || source.calls.Load() != 2 {
				t.Fatal("fresh result not saved for ordinary requests")
			}
		})
	}
}

func TestRefreshDoesNotPublishOfficialOnlyDataWhenJavDBFails(t *testing.T) {
	official := &sourceStub{id: "fanza", fetch: func(_ context.Context, code string) (domain.MovieMetadata, error) {
		return completeFixture("fanza", code), nil
	}}
	catalogue := &sourceStub{id: "javdb", fetch: func(context.Context, string) (domain.MovieMetadata, error) {
		return domain.MovieMetadata{}, context.DeadlineExceeded
	}}
	s := newTestService(t, official, catalogue)
	m, err := s.Resolve(t.Context(), domain.MovieRef{Code: "IPZZ-960", Refresh: true})
	if err == nil || m.Detail.Code != "" {
		t.Fatalf("partial rebuild accepted: %+v %v", m, err)
	}
	if _, retry := domain.RetryDelay(err); !retry {
		t.Fatalf("source failure cannot retry: %v", err)
	}
}

func TestSourceFailuresExposeOnlySourceNamesAndKeepDiagnosticCauses(t *testing.T) {
	cause := &domain.RetryError{Cause: errors.New("Get https://source.example/private/path: proxyconnect tcp: connection refused")}
	for _, names := range [][]string{{"pacopacomama"}, {"pacopacomama", "heyzo"}} {
		t.Run(strings.Join(names, "+"), func(t *testing.T) {
			var sources []Source
			for _, name := range names {
				sources = append(sources, &sourceStub{id: name, fetch: func(context.Context, string) (domain.MovieMetadata, error) {
					return domain.MovieMetadata{}, cause
				}})
			}
			sources = append(sources, &sourceStub{id: "javdb", fetch: func(context.Context, string) (domain.MovieMetadata, error) {
				return domain.MovieMetadata{}, ErrNotFound
			}})
			s := newTestService(t, sources...)
			_, err := s.Resolve(t.Context(), domain.MovieRef{Code: "042126_100", Refresh: true})
			want := "刮削来源查询失败: " + strings.Join(names, "、") + "，请检查网络和代理设置"
			if domain.PublicMessage(err) != want || !errors.Is(err, cause) || !strings.Contains(err.Error(), "proxyconnect tcp") {
				t.Fatalf("incorrect public message or lost diagnostics: %v", err)
			}
			if _, retry := domain.RetryDelay(err); !retry {
				t.Fatal("simplified message changed retry classification")
			}
		})
	}
}

func completeFixture(provider, code string) domain.MovieMetadata {
	m := fixture(provider, code, provider+" title")
	m.Detail.ReleaseDate, m.Detail.Duration = "2026-01-01", 120
	m.Detail.Actors = []domain.Actor{{Provider: provider, ID: "actor", Name: "Actor"}}
	m.Detail.Maker = &domain.Maker{Provider: provider, ID: "maker", Name: "Studio"}
	m.Detail.Tags = []domain.Tag{{Provider: provider, ID: "tag", Name: "Genre"}}
	m.Images = []domain.ImageCandidate{{Provider: provider, URL: "cover", Role: "cover"}, {Provider: provider, URL: "preview", Role: "preview"}}
	return m
}

func TestCompleteOfficialMetadataStillQueriesJavDBAndReusesCache(t *testing.T) {
	primary := &sourceStub{id: "fanza", fetch: func(_ context.Context, code string) (domain.MovieMetadata, error) {
		return completeFixture("fanza", code), nil
	}}
	fallback := &sourceStub{id: "javdb", fetch: func(_ context.Context, code string) (domain.MovieMetadata, error) {
		return completeFixture("javdb", code), nil
	}}
	s := newTestService(t, primary, fallback)
	for range 2 {
		m, err := s.Resolve(t.Context(), domain.MovieRef{Code: "ABP-123"})
		if err != nil || m.Detail.Title != "javdb title" || m.Detail.ID != "ABP-123" || m.Detail.Actors[0].Provider != "javdb" || m.Detail.Tags[0].Provider != "javdb" {
			t.Fatalf("result=%+v err=%v", m, err)
		}
	}
	if primary.calls.Load() != 1 || fallback.calls.Load() != 1 {
		t.Fatal("catalogue identity was skipped or source cache was not reused")
	}
	for range 2 {
		if _, err := s.Fallback(t.Context(), domain.MovieRef{Code: "ABP-123"}); err != nil {
			t.Fatal(err)
		}
	}
	if fallback.calls.Load() != 1 {
		t.Fatal("image fallback did not reuse source cache")
	}
	if err := s.UpdateSettings(t.Context(), []SourceSetting{{ID: "fanza", Enabled: true}, {ID: "javdb", Enabled: false}}); err == nil {
		t.Fatal("fallback accepted a user-controlled switch")
	}
}

func TestJavDBMetadataTakesPriorityAndReturnsItsConfirmedIdentity(t *testing.T) {
	var order []string
	primary := &sourceStub{id: "fanza", fetch: func(_ context.Context, code string) (domain.MovieMetadata, error) {
		order = append(order, "fanza")
		return fixture("fanza", code, "Official title"), nil
	}}
	fallback := &sourceStub{id: "javdb", fetch: func(_ context.Context, code string) (domain.MovieMetadata, error) {
		order = append(order, "javdb")
		m := completeFixture("javdb", code)
		m.Detail.Sources[0].ID = "catalogue-id"
		return m, nil
	}}
	s := newTestService(t, primary, fallback)
	m, err := s.Resolve(t.Context(), domain.MovieRef{Code: "ABP-123"})
	if err != nil || !reflect.DeepEqual(order, []string{"fanza", "javdb"}) || m.Detail.Title != "javdb title" || m.Detail.ID != "catalogue-id" || m.Detail.FieldSources["title"] != "javdb" || m.Detail.FieldSources["actors"] != "javdb" {
		t.Fatalf("merge=%+v order=%v err=%v", m, order, err)
	}
}

func TestOfficialMetadataRemainsUsableWithoutJavDB(t *testing.T) {
	for _, failure := range []error{ErrNotFound, errors.New("network unavailable")} {
		t.Run(failure.Error(), func(t *testing.T) {
			primary := &sourceStub{id: "fanza", fetch: func(_ context.Context, code string) (domain.MovieMetadata, error) {
				return completeFixture("fanza", code), nil
			}}
			catalogue := &sourceStub{id: "javdb", fetch: func(context.Context, string) (domain.MovieMetadata, error) {
				return domain.MovieMetadata{}, failure
			}}
			s := newTestService(t, primary, catalogue)
			m, err := s.Resolve(t.Context(), domain.MovieRef{Code: "ABP-123"})
			if err != nil || m.Detail.ID != "" || m.Detail.Title != "fanza title" || len(m.Detail.PreviewImages) != 1 || m.Detail.Tags[0].Provider != "fanza" {
				t.Fatalf("official metadata lost: %+v %v", m, err)
			}
		})
	}
}

func TestStrongerJavDBIdentityWinsBeforeAnySourceDowngrade(t *testing.T) {
	primary := &sourceStub{id: "fanza", fetch: func(_ context.Context, code string) (domain.MovieMetadata, error) {
		if code == "118ABP-123" {
			return domain.MovieMetadata{}, ErrNotFound
		}
		return completeFixture("fanza", code), nil
	}}
	fallback := &sourceStub{id: "javdb", fetch: func(_ context.Context, code string) (domain.MovieMetadata, error) {
		return completeFixture("javdb", code), nil
	}}
	s := newTestService(t, primary, fallback)
	m, err := s.Resolve(t.Context(), domain.MovieRef{Code: "118ABP-123"})
	if err != nil || m.Detail.Code != "118ABP-123" || m.Detail.Sources[0].Provider != "javdb" || primary.calls.Load() != 1 {
		t.Fatalf("strong identity lost: %+v %v", m, err)
	}
}

func TestPrimaryFailureCanUseSameIdentityFallbackButNeverWeakerIdentity(t *testing.T) {
	primary := &sourceStub{id: "fanza", fetch: func(context.Context, string) (domain.MovieMetadata, error) {
		return domain.MovieMetadata{}, errors.New("network unavailable")
	}}
	fallback := &sourceStub{id: "javdb", fetch: func(_ context.Context, code string) (domain.MovieMetadata, error) {
		return completeFixture("javdb", code), nil
	}}
	s := newTestService(t, primary, fallback)
	if _, err := s.Resolve(t.Context(), domain.MovieRef{Code: "118ABP-123"}); err != nil {
		t.Fatal(err)
	}
	fallback.fetch = func(context.Context, string) (domain.MovieMetadata, error) {
		return domain.MovieMetadata{}, ErrNotFound
	}
	if _, err := s.Resolve(t.Context(), domain.MovieRef{Code: "118ABP-124"}); err == nil {
		t.Fatal("network failure became a weaker match")
	}
	if primary.calls.Load() != 2 || fallback.calls.Load() != 2 {
		t.Fatal("weaker layers were queried after network failure")
	}
}

func TestFANZAFirstAndJavDBLastSurviveSavedSettings(t *testing.T) {
	old := newTestService(t, &sourceStub{id: "fc2"})
	if err := old.UpdateSettings(t.Context(), []SourceSetting{{ID: "fc2", Enabled: false}}); err != nil {
		t.Fatal(err)
	}
	s, err := New(t.Context(), old.db, &sourceStub{id: "fanza"}, &sourceStub{id: "fc2"}, &sourceStub{id: "javdb"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	want := []SourceSetting{{ID: "fanza", Enabled: true}, {ID: "fc2", Enabled: false}}
	if !reflect.DeepEqual(s.Settings(), want) {
		t.Fatalf("order=%v", s.Settings())
	}
	if err := s.UpdateSettings(t.Context(), []SourceSetting{want[1], want[0]}); err == nil {
		t.Fatal("FANZA can lose its priority")
	}
}

func TestJavDBIsHiddenAndAlwaysEnabled(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := database.SaveSetting(t.Context(), store.Client, "metadata.sources", []SourceSetting{
		{ID: "javdb", Enabled: false}, {ID: "fanza", Enabled: false},
	}); err != nil {
		t.Fatal(err)
	}
	primary := &sourceStub{id: "fanza", fetch: func(context.Context, string) (domain.MovieMetadata, error) {
		t.Error("disabled source queried")
		return domain.MovieMetadata{}, ErrNotFound
	}}
	fallback := &sourceStub{id: "javdb", fetch: func(_ context.Context, code string) (domain.MovieMetadata, error) {
		return completeFixture("javdb", code), nil
	}}
	s, err := New(t.Context(), store.Client, fallback, primary)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	want := []SourceSetting{{ID: "fanza", Enabled: false}}
	if !reflect.DeepEqual(s.Settings(), want) {
		t.Fatalf("public settings include fallback: %v", s.Settings())
	}
	if err := s.UpdateSettings(t.Context(), want); err != nil {
		t.Fatal(err)
	}
	for _, resolve := range []func(context.Context, domain.MovieRef) (domain.MovieMetadata, error){s.Resolve, s.Fallback} {
		m, err := resolve(t.Context(), domain.MovieRef{Code: "ABP-123"})
		if err != nil || m.Detail.Sources[0].Provider != "javdb" {
			t.Fatalf("fallback disabled by settings: %+v %v", m, err)
		}
	}
}

func TestKnownJavDBIDCanRecoverFromCachedSearchMiss(t *testing.T) {
	source := &sourceStub{id: "javdb"}
	source.fetch = func(_ context.Context, code string) (domain.MovieMetadata, error) {
		if source.calls.Load() == 1 {
			return domain.MovieMetadata{}, ErrNotFound
		}
		m := completeFixture("javdb", code)
		m.Detail.ID = "known"
		m.Detail.Sources[0].ID = "known"
		return m, nil
	}
	s := newTestService(t, source)
	if _, err := s.Resolve(t.Context(), domain.MovieRef{Code: "ABP-123"}); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	for range 2 {
		m, err := s.Fallback(t.Context(), domain.MovieRef{Code: "ABP-123", JavDBID: "known"})
		if err != nil || m.Detail.ID != "known" {
			t.Fatalf("cached search miss blocked ID lookup: %+v %v", m, err)
		}
	}
	if source.calls.Load() != 2 {
		t.Fatal("known identity was not cached")
	}
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

func TestSourceSettingsFollowRegisteredSources(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	saved := []SourceSetting{{ID: "removed", Enabled: true}, {ID: "retained", Enabled: false}}
	if err := database.SaveSetting(t.Context(), store.Client, "metadata.sources", saved); err != nil {
		t.Fatal(err)
	}
	s, err := New(t.Context(), store.Client, &sourceStub{id: "added"}, &sourceStub{id: "retained"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	want := []SourceSetting{{ID: "retained", Enabled: false}, {ID: "added", Enabled: true}}
	if got := s.Settings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("settings = %+v, want %+v", got, want)
	}
	if err := s.UpdateSettings(t.Context(), saved); err == nil {
		t.Fatal("removed source can still be enabled")
	}
	if err := s.UpdateSettings(t.Context(), want); err != nil {
		t.Fatal(err)
	}
}

func TestResolveWithoutJavDBMergesInPriorityOrderAndPersistsCache(t *testing.T) {
	primary := &sourceStub{id: "primary", fetch: func(_ context.Context, code string) (domain.MovieMetadata, error) {
		m := fixture("primary", code, "Primary title")
		m.Detail.Actors = []domain.Actor{{Provider: "primary", ID: "a", Name: "Same name"}}
		return m, nil
	}}
	secondary := &sourceStub{id: "secondary", fetch: func(_ context.Context, code string) (domain.MovieMetadata, error) {
		m := fixture("secondary", code, "Secondary title")
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
		if m.Detail.ID != "" || m.Detail.Title != "Primary title" || len(m.Detail.Actors) != 1 || m.Detail.Actors[0].Provider != "primary" || m.Detail.RatingSource != "secondary" || m.Detail.RatingMax != 10 || len(m.Images) != 1 {
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
