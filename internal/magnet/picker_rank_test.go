package magnet

import (
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
)

func TestPickerScoringTiersAndInferredWeights(t *testing.T) {
	p := NewPicker(Preferences{Subtitle: PreferencePreferred, HD: PreferencePreferred, Uncensored: UncensoredPreferred})

	verifiedSub := domain.Magnet{Hash: "verified_sub", Name: "SSIS-001", HasSubtitle: true, Tags: []string{domain.MagnetTagSubtitle}, Size: 1000}
	inferredSub := domain.Magnet{Hash: "inferred_sub", Name: "SSIS-001 中文字幕版", Size: 2000, Tags: []string{domain.MagnetTagSubtitle}, Inferred: true}
	if best, _ := p.Pick([]domain.Magnet{inferredSub, verifiedSub}); best.Hash != "verified_sub" {
		t.Fatalf("verified subtitle must outrank inferred, got %s", best.Hash)
	}

	subOnly := domain.Magnet{Hash: "sub_only", Name: "SSIS-001", HasSubtitle: true, Tags: []string{domain.MagnetTagSubtitle}, Size: 1000}
	hdOnly := domain.Magnet{Hash: "hd_only", Name: "SSIS-001", HD: true, Tags: []string{domain.MagnetTagHD}, Size: 5000}
	if best, _ := p.Pick([]domain.Magnet{hdOnly, subOnly}); best.Hash != "sub_only" {
		t.Fatalf("subtitle must outrank HD, got %s", best.Hash)
	}

	uncenOnly := domain.Magnet{Hash: "uncen_only", Name: "SSIS-001 无码流出", Size: 5000, Tags: []string{domain.MagnetTagUncensored, domain.MagnetTagCracked}, Inferred: true}
	if best, _ := p.Pick([]domain.Magnet{uncenOnly, hdOnly}); best.Hash != "hd_only" {
		t.Fatalf("HD must outrank uncensored, got %s", best.Hash)
	}
	inferredHD := domain.Magnet{Hash: "inferred_hd", Name: "SSIS-001 4K", Size: 10000, Tags: []string{domain.MagnetTag4K}, Inferred: true}
	if best, _ := p.Pick([]domain.Magnet{inferredHD, hdOnly}); best.Hash != "hd_only" {
		t.Fatalf("verified HD must outrank inferred 4K, got %s", best.Hash)
	}
}

// A site-verified subtitle keeps its full weight even when the name also
// matches the subtitle heuristics and carries other inferred markers.
func TestPickerVerifiedSubtitleSurvivesInferredTags(t *testing.T) {
	p := NewPicker(DefaultPreferences())
	verified4K := domain.Magnet{Hash: "a", Name: "SSIS-001-UC 4K", HasSubtitle: true, HD: true,
		Tags: []string{domain.MagnetTagSubtitle, domain.MagnetTagHD, domain.MagnetTag4K, domain.MagnetTagUncensored}, Inferred: true, Sources: []string{domain.MagnetSourceJavDB}, Size: 1000}
	verifiedPlain := domain.Magnet{Hash: "b", Name: "SSIS-001", HasSubtitle: true, HD: true,
		Tags: []string{domain.MagnetTagSubtitle, domain.MagnetTagHD}, Sources: []string{domain.MagnetSourceJavDB}, Size: 500}
	best, ok := p.Pick([]domain.Magnet{verifiedPlain, verified4K})
	if !ok || best.Hash != "a" {
		t.Fatalf("expected the larger verified magnet a, got %s", best.Hash)
	}
}

func TestPickerTieBreaking(t *testing.T) {
	p := NewPicker(DefaultPreferences())
	base := domain.Magnet{Name: "SSIS-001", HasSubtitle: true, HD: true}

	smaller, larger := base, base
	smaller.Hash, smaller.Size = "h1", 1000
	larger.Hash, larger.Size = "h2", 2000
	if best, _ := p.Pick([]domain.Magnet{smaller, larger}); best.Hash != "h2" {
		t.Fatalf("larger size must win, got %s", best.Hash)
	}

	fewer, more := base, base
	fewer.Hash, fewer.Size, fewer.FilesCount = "f1", 1000, 1
	more.Hash, more.Size, more.FilesCount = "f2", 1000, 5
	if best, _ := p.Pick([]domain.Magnet{fewer, more}); best.Hash != "f2" {
		t.Fatalf("more files must win, got %s", best.Hash)
	}

	javbus, javdb := base, base
	javbus.Hash, javbus.Size, javbus.Sources = "b1", 1000, []string{domain.MagnetSourceJavBus}
	javdb.Hash, javdb.Size, javdb.Sources = "d1", 1000, []string{domain.MagnetSourceJavDB}
	if best, _ := p.Pick([]domain.Magnet{javbus, javdb}); best.Hash != "d1" {
		t.Fatalf("JavDB-listed magnet must win, got %s", best.Hash)
	}
}
