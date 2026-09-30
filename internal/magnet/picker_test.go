package magnet

import (
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
)

func TestPickerEmpty(t *testing.T) {
	p := NewPicker(DefaultPreferences())
	if _, ok := p.Pick(nil); ok {
		t.Fatal("expected pick on nil to return false")
	}
	if _, ok := p.Pick([]domain.Magnet{}); ok {
		t.Fatal("expected pick on empty slice to return false")
	}
}

func TestPreferencesValidate(t *testing.T) {
	if err := DefaultPreferences().Validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
	if err := (Preferences{}).Normalized().Validate(); err != nil {
		t.Fatalf("normalized empty preferences must validate: %v", err)
	}
	err := (Preferences{Subtitle: "sometimes", HD: PreferenceAny, Uncensored: UncensoredAny}).Validate()
	if !domain.IsKind(err, domain.KindInvalid) {
		t.Fatalf("expected KindInvalid, got %v", err)
	}
}

func TestPickerSubtitleRequired(t *testing.T) {
	p := NewPicker(Preferences{Subtitle: PreferenceRequired, HD: PreferenceAny, Uncensored: UncensoredAny})
	magnets := []domain.Magnet{
		{Hash: "h1", Name: "SSIS-001 No Sub", Size: 5000},
		{Hash: "h2", Name: "SSIS-001 With Sub", HasSubtitle: true, Tags: []string{domain.MagnetTagSubtitle}, Size: 2000},
	}
	best, ok := p.Pick(magnets)
	if !ok || best.Hash != "h2" {
		t.Fatalf("expected h2, got %v ok=%v", best.Hash, ok)
	}
	if _, ok := p.Pick(magnets[:1]); ok {
		t.Fatal("expected no match when subtitle is required and missing")
	}
	// An inferred subtitle satisfies the requirement too.
	if _, ok := p.Pick([]domain.Magnet{{Hash: "h3", Name: "SSIS-001-C", Tags: []string{domain.MagnetTagSubtitle}, Inferred: true}}); !ok {
		t.Fatal("expected an inferred subtitle to satisfy the requirement")
	}
}

func TestPickerHDRequired(t *testing.T) {
	p := NewPicker(Preferences{Subtitle: PreferenceAny, HD: PreferenceRequired, Uncensored: UncensoredAny})
	magnets := []domain.Magnet{
		{Hash: "h1", Name: "SSIS-001 SD", Size: 5000},
		{Hash: "h2", Name: "SSIS-001 HD", HD: true, Tags: []string{domain.MagnetTagHD}, Size: 3000},
	}
	best, ok := p.Pick(magnets)
	if !ok || best.Hash != "h2" {
		t.Fatalf("expected h2, got %v ok=%v", best.Hash, ok)
	}
	if _, ok := p.Pick(magnets[:1]); ok {
		t.Fatal("expected no match when HD is required and missing")
	}
	if _, ok := p.Pick([]domain.Magnet{{Hash: "h3", Name: "SSIS-001 4K", Tags: []string{domain.MagnetTag4K}, Inferred: true}}); !ok {
		t.Fatal("expected inferred 4K to satisfy the HD requirement")
	}
}

func TestPickerUncensoredExclude(t *testing.T) {
	p := NewPicker(Preferences{Subtitle: PreferenceAny, HD: PreferenceAny, Uncensored: UncensoredExclude})
	magnets := []domain.Magnet{
		{Hash: "h1", Name: "SSIS-001-UC 无码破解", Size: 5000, Tags: []string{domain.MagnetTagSubtitle, domain.MagnetTagUncensored, domain.MagnetTagCracked}, Inferred: true},
		{Hash: "h2", Name: "SSIS-001.Leaked.720p", Size: 4000, Tags: []string{domain.MagnetTagCracked}, Inferred: true},
		{Hash: "h3", Name: "SSIS-001 Standard", Size: 3000},
	}
	best, ok := p.Pick(magnets)
	if !ok || best.Hash != "h3" {
		t.Fatalf("expected h3, got %v ok=%v", best.Hash, ok)
	}
	if _, ok := p.Pick(magnets[:2]); ok {
		t.Fatal("expected no match when every magnet is uncensored and uncensored is excluded")
	}
}

func TestPickerUncensoredRequired(t *testing.T) {
	p := NewPicker(Preferences{Subtitle: PreferenceAny, HD: PreferenceAny, Uncensored: UncensoredRequired})
	magnets := []domain.Magnet{
		{Hash: "h1", Name: "SSIS-001 Standard", Size: 5000},
		{Hash: "h2", Name: "SSIS-001-UC 破解版", Size: 3000, Tags: []string{domain.MagnetTagSubtitle, domain.MagnetTagUncensored, domain.MagnetTagCracked}, Inferred: true},
	}
	best, ok := p.Pick(magnets)
	if !ok || best.Hash != "h2" {
		t.Fatalf("expected h2, got %v ok=%v", best.Hash, ok)
	}
}
