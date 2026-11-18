package codeid

import (
	"slices"
	"testing"
)

func TestSubtitleSuffixVariants(t *testing.T) {
	cases := []struct {
		code string
		base string
	}{
		{"MEYD-589CH", "MEYD-589"},
		{"NTRH-016CH", "NTRH-016"},
	}
	for _, testCase := range cases {
		if got := withoutSubtitleSuffix(testCase.code); got != testCase.base {
			t.Fatalf("%s base = %q, want %q", testCase.code, got, testCase.base)
		}
		if !slices.Contains(Queries(testCase.code), testCase.base) {
			t.Fatalf("%s queries missing base: %v", testCase.code, Queries(testCase.code))
		}
		found := false
		for _, layer := range Layers(testCase.code) {
			if slices.Contains(layer, testCase.base) {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s layers missing base: %v", testCase.code, Layers(testCase.code))
		}
		if !IsFormatEquivalent(testCase.code, testCase.base) || !IsFormatEquivalent(testCase.base, testCase.code) {
			t.Fatalf("%s not equivalent to base %s", testCase.code, testCase.base)
		}
	}
}

func TestSubtitleSuffixDoesNotStripSerialLetters(t *testing.T) {
	if IsFormatEquivalent("ABC-123A", "ABC-123") {
		t.Fatal("serial letter A must not be treated as a subtitle suffix")
	}
	if IsFormatEquivalent("DVDMS-669C", "DVDMS-669") {
		t.Fatal("a bare C stays a serial identity, not a subtitle suffix")
	}
}
