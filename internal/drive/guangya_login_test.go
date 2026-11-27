package drive

import "testing"

func TestNormalizeGuangyaPhone(t *testing.T) {
	cases := map[string]string{
		"13800138000":       "+8613800138000",
		"138 0013 8000":     "+8613800138000",
		"138-0013-8000":     "+8613800138000",
		"8613800138000":     "+8613800138000",
		"008613800138000":   "+8613800138000",
		"+86 13800138000":   "+8613800138000",
		"+14155550123":      "+14155550123",
		"  +8613800138000 ": "+8613800138000",
		"":                  "",
	}
	for input, want := range cases {
		if got := normalizeGuangyaPhone(input); got != want {
			t.Errorf("normalizeGuangyaPhone(%q) = %q, want %q", input, got, want)
		}
	}
}
