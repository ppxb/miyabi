package codeid

import "testing"

func TestMatchKeyPreservesCatalogueVariants(t *testing.T) {
	for _, tt := range []struct{ code, want string }{
		{"abc00123", "ABC-123"},
		{"326IHD-005", "IHD-5"},
		{"ABC-000", "ABC-0"},
		{"FJIN-00106A", "FJIN-00106A"},
		{"1pondo-060326_001", "060326-001"},
		{"060326_001", "060326_001"},
		{"FC2-00123", "FC2-PPV-00123"},
		{"HEYDOUGA-4017-00001", "HEYDOUGA-4017-00001"},
		{"SCUTE-1575-ITSUKI-02", "SCUTE-1575-ITSUKI-02"},
		{"21Studio.26.09.05-scene2", "21STUDIO.26.09.05-SCENE2"},
		{"", ""},
	} {
		if got := MatchKey(tt.code); got != tt.want {
			t.Errorf("MatchKey(%q)=%q, want %q", tt.code, got, tt.want)
		}
	}
}

func FuzzEquivalentCodesShareMatchKey(f *testing.F) {
	for _, pair := range [][2]string{
		{"ABC00123", "ABC-123"}, {"326IHD-005", "IHD-5"},
		{"1PONDO-060326-001", "060326-001"}, {"FC2-123", "FC2-PPV-123"},
		{"259LUXU-1899", "999LUXU-1899"}, {"", ""},
	} {
		f.Add(pair[0], pair[1])
	}
	f.Fuzz(func(t *testing.T, a, b string) {
		if IsEquivalent(a, b) && MatchKey(a) != MatchKey(b) {
			t.Fatalf("equivalent codes would be excluded by the index: %q, %q", a, b)
		}
	})
}
