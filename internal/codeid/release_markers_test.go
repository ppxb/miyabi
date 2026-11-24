package codeid

import (
	"slices"
	"testing"
)

// TestLTXParse covers leak/uncensored LTX markers: both separator and glued forms
// must reduce to the base catalogue number at parse (filename) time.
func TestLTXParse(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"SSIS-001-LTX.mp4", "SSIS-001"},
		{"ssis001-LTX.mp4", "SSIS-001"},
		{"SSIS-001-LTX-4K.mp4", "SSIS-001"},
		{"KNB-M014-LTX.mp4", "KNB-M014"},
		{"SSIS-001LTX.mp4", "SSIS-001"}, // glued to the serial
		{"ssis001LTX.mp4", "SSIS-001"},
	}
	for _, tc := range cases {
		if got, ok := Parse(tc.input); !ok || got != tc.want {
			t.Errorf("Parse(%q) = %q,%v; want %q", tc.input, got, ok, tc.want)
		}
	}
}

// TestLTXLayers ensures an LTX marker still present in a code (dash or glued) is
// offered as a lower-priority lookup candidate so the base code can resolve.
func TestLTXLayers(t *testing.T) {
	cases := []struct {
		code string
		base string
	}{
		{"SSIS-001-LTX", "SSIS-001"},
		{"KNB-M014-LTX", "KNB-M014"},
		{"SSIS-001LTX", "SSIS-001"},
	}
	for _, tc := range cases {
		found := false
		for _, layer := range Layers(tc.code) {
			if slices.Contains(layer, tc.base) {
				found = true
			}
		}
		if !found {
			t.Errorf("Layers(%q) missing base %q: %v", tc.code, tc.base, Layers(tc.code))
		}
	}
}

// TestAttachedCIsLookupFallbackOnly keeps the existing guarantee that a bare C is
// a serial letter (distinct identity, preserved at parse time), while still
// offering the base code as a relaxed lookup candidate for Chinese-subtitle C.
func TestAttachedCIsLookupFallbackOnly(t *testing.T) {
	if got, _ := Parse("FJIN-106C.mp4"); got != "FJIN-106C" {
		t.Errorf("Parse(FJIN-106C) = %q; want FJIN-106C (serial C must be preserved)", got)
	}
	if IsFormatEquivalent("FJIN-106C", "FJIN-106") {
		t.Error("FJIN-106C must stay distinct from FJIN-106")
	}
	if IsFormatEquivalent("DVDMS-669C", "DVDMS-669") {
		t.Error("DVDMS-669C must stay distinct from DVDMS-669")
	}
	for _, tc := range []struct{ in, base string }{
		{"FJIN-106C", "FJIN-106"},
		{"SSIS-748C", "SSIS-748"},
		{"DVDMS-669C", "DVDMS-669"},
	} {
		found := false
		for _, layer := range Layers(tc.in) {
			if slices.Contains(layer, tc.base) {
				found = true
			}
		}
		if !found {
			t.Errorf("Layers(%q) missing subtitle-C fallback %q: %v", tc.in, tc.base, Layers(tc.in))
		}
	}
}
