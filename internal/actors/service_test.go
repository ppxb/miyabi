package actors

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsPlaceholderAvatar(t *testing.T) {
	if !isPlaceholderAvatar(imageSource{ref: "https://example.invalid/nowprinting.gif"}, nil) {
		t.Fatal("nowprinting URL not detected")
	}
	for _, name := range []string{"placeholder_gfriends.png", "placeholder_dmm.gif"} {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if !isPlaceholderAvatar(imageSource{provider: "gfriends", ref: "Content/x.jpg"}, data) {
			t.Fatalf("%s not detected as a placeholder", name)
		}
	}
	if isPlaceholderAvatar(imageSource{ref: "https://example.invalid/real.jpg"}, []byte("\xff\xd8\xff\xe0real")) {
		t.Fatal("a real image was detected as a placeholder")
	}
}

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
