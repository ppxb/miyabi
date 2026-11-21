package actors

import (
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
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

func TestKeyStableForEquivalentNames(t *testing.T) {
	if domain.ActorNameKey(normalizeName("七ツ森 りり")) != domain.ActorNameKey(normalizeName("七ツ森りり")) {
		t.Fatal("spacing variant should map to same key")
	}
	if domain.ActorNameKey(normalizeName("Actor")) == domain.ActorNameKey(normalizeName("Other")) {
		t.Fatal("different names must differ")
	}
	if len(domain.ActorNameKey("x")) != 40 {
		t.Fatalf("key should be sha1 hex (40 chars), got %d", len(domain.ActorNameKey("x")))
	}
}
