package actors

import (
	"testing"
)

func TestNormalizeMergesWhitespaceAndCase(t *testing.T) {
	cases := map[string]string{
		" 七ツ森 りり ": "七ツ森りり",
		"七ツ森りり":    "七ツ森りり",
		"ABC":      "abc",
		"　全角 空 白":  "全角空白",
	}
	for in, want := range cases {
		if got := normalizeName(in); got != want {
			t.Fatalf("normalizeName(%q)=%q want %q", in, got, want)
		}
	}
}
