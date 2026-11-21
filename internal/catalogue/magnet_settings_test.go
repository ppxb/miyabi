package catalogue

import (
	"context"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
)

type namedMagnetSource struct{ name string }

func (s namedMagnetSource) Name() string { return s.name }

func (s namedMagnetSource) Find(context.Context, domain.MovieRef) ([]domain.Magnet, error) {
	return nil, nil
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

	updated := []MagnetSourceSetting{
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
	if err := reopened.UpdateMagnetSourceSettings(t.Context(), []MagnetSourceSetting{{ID: "unknown", Enabled: true}}); err == nil {
		t.Fatal("expected validation error for unknown magnet source")
	}
}
