package service

import (
	"testing"

	"github.com/ppxb/miyabi/internal/database"
)

func TestPreferencesPersistBothNSFWModes(t *testing.T) {
	directory := t.TempDir()
	store, err := database.Open(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	settings := NewSettingService(store.Client)
	initial, err := settings.Preferences(t.Context())
	if err != nil || !initial.NSFWMode {
		t.Fatalf("initial preferences = %+v, error = %v", initial, err)
	}
	for _, enabled := range []bool{false, true, false} {
		if _, err := settings.SavePreferences(t.Context(), Preferences{NSFWMode: enabled}); err != nil {
			t.Fatal(err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		store, err = database.Open(t.Context(), directory)
		if err != nil {
			t.Fatal(err)
		}
		settings = NewSettingService(store.Client)
		actual, err := settings.Preferences(t.Context())
		if err != nil || actual.NSFWMode != enabled {
			t.Fatalf("reopened preferences = %+v, want NSFW %t, error = %v", actual, enabled, err)
		}
	}
	store.Close()
}
