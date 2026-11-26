package catalogue

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
)

type namedMagnetSource struct{ name string }

func (s namedMagnetSource) Name() string { return s.name }

func (s namedMagnetSource) Find(context.Context, domain.MovieRef) ([]domain.Magnet, error) {
	return nil, nil
}

type countingMagnetSource struct {
	name    string
	magnets []domain.Magnet
	calls   atomic.Int32
}

func (s *countingMagnetSource) Name() string { return s.name }

func (s *countingMagnetSource) Find(context.Context, domain.MovieRef) ([]domain.Magnet, error) {
	s.calls.Add(1)
	return s.magnets, nil
}

func TestMagnetSourceSettingsPersistAndRestore(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	primary := &stubProviderWithMagnets{}
	sukebei := namedMagnetSource{name: domain.MagnetSourceSukebei}
	projectjav := namedMagnetSource{name: domain.MagnetSourceProjectJav}

	service, err := NewWithClients(t.Context(), store.Client, primary, nil, &stubLocalState{}, sukebei, projectjav)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	defer service.Close()

	initial := service.MagnetSourceSettings()
	if len(initial) != 2 || initial[0].ID != domain.MagnetSourceSukebei || initial[1].ID != domain.MagnetSourceProjectJav {
		t.Fatalf("initial settings = %+v", initial)
	}
	if !initial[0].Enabled || !initial[1].Enabled {
		t.Fatalf("magnet sources should default to enabled: %+v", initial)
	}

	updated := []domain.SourceToggle{
		{ID: domain.MagnetSourceSukebei, Enabled: true},
		{ID: domain.MagnetSourceProjectJav, Enabled: false},
	}
	if err := service.UpdateMagnetSourceSettings(t.Context(), updated); err != nil {
		t.Fatalf("update settings: %v", err)
	}

	reopened, err := NewWithClients(t.Context(), store.Client, primary, nil, &stubLocalState{}, sukebei, projectjav)
	if err != nil {
		t.Fatalf("reopen service: %v", err)
	}
	defer reopened.Close()
	restored := reopened.MagnetSourceSettings()
	if len(restored) != 2 || restored[1].ID != domain.MagnetSourceProjectJav || restored[1].Enabled {
		t.Fatalf("restored settings = %+v, want projectjav disabled", restored)
	}

	// Reject unknown sources.
	if err := reopened.UpdateMagnetSourceSettings(t.Context(), []domain.SourceToggle{{ID: "unknown", Enabled: true}}); err == nil {
		t.Fatal("expected validation error for unknown magnet source")
	}
}

func TestMagnetSourceSettingsInvalidateCachedMagnets(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	projectjav := &countingMagnetSource{
		name:    domain.MagnetSourceProjectJav,
		magnets: []domain.Magnet{{Hash: "3333333333333333333333333333333333333333", Sources: []string{domain.MagnetSourceProjectJav}}},
	}
	service, err := NewWithClients(t.Context(), store.Client, &stubProviderWithMagnets{}, nil, &stubLocalState{}, nil, projectjav)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	defer service.Close()

	first, err := service.MagnetsByCode(t.Context(), "SSIS-001", "")
	if err != nil || len(first) != 1 {
		t.Fatalf("initial magnets = %+v, err = %v", first, err)
	}
	if _, err := service.MagnetsByCode(t.Context(), "SSIS-001", ""); err != nil {
		t.Fatal(err)
	}
	if calls := projectjav.calls.Load(); calls != 1 {
		t.Fatalf("cached lookup reached ProjectJav %d times", calls)
	}

	if err := service.UpdateMagnetSourceSettings(t.Context(), []domain.SourceToggle{{ID: domain.MagnetSourceProjectJav, Enabled: false}}); err != nil {
		t.Fatalf("disable source: %v", err)
	}
	after, err := service.MagnetsByCode(t.Context(), "SSIS-001", "")
	if err != nil || len(after) != 0 {
		t.Fatalf("disabled source still returned magnets: %+v, err = %v", after, err)
	}
	if calls := projectjav.calls.Load(); calls != 1 {
		t.Fatalf("disabled source was queried %d times", calls)
	}
}
