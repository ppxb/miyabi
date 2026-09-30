package javbus

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func fixtureFile(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return content
}

func TestParseMagnetsHTML_SSIS001(t *testing.T) {
	content := fixtureFile(t, "magnets_ssis-001.html")
	magnets, err := parseMagnetsHTML(string(content))
	if err != nil {
		t.Fatalf("unexpected error parsing magnets HTML: %v", err)
	}

	if len(magnets) != 43 {
		t.Fatalf("expected 43 magnets, got %d", len(magnets))
	}

	hdCount := 0
	subCount := 0
	for _, m := range magnets {
		if m.HD {
			hdCount++
		}
		if m.HasSubtitle {
			subCount++
		}
		if len(m.Sources) != 1 || m.Sources[0] != "javbus" {
			t.Errorf("expected source to be javbus, got %v", m.Sources)
		}
		if m.HD != slices.Contains(m.Tags, "高清") || m.HasSubtitle != slices.Contains(m.Tags, "字幕") {
			t.Errorf("display tags disagree with site flags: %+v", m)
		}
	}

	if hdCount != 26 {
		t.Errorf("expected 26 HD magnets, got %d", hdCount)
	}
	if subCount != 8 {
		t.Errorf("expected 8 subtitle magnets, got %d", subCount)
	}

	first := magnets[0]
	if first.Hash != "30291c52bb72d46affc1574ec01a4e16fc28a292" {
		t.Errorf("unexpected first magnet hash: %s", first.Hash)
	}
	if first.Name != "SSIS-001" {
		t.Errorf("unexpected first magnet name: %s", first.Name)
	}
	if first.CreatedAt != "2025-10-28" {
		t.Errorf("unexpected first magnet created_at: %s", first.CreatedAt)
	}
	if first.HasSubtitle || first.HD {
		t.Errorf("first magnet should not have subtitle or HD: %+v", first)
	}

	second := magnets[1]
	if second.Hash != "fcef4eb87ac04a85c5285f8377b2856e9e40b9f1" {
		t.Errorf("unexpected second magnet hash: %s", second.Hash)
	}
	if second.Name != "SSIS-001-UC" {
		t.Errorf("unexpected second magnet name: %s", second.Name)
	}
	if !second.HD || second.HasSubtitle {
		t.Errorf("second magnet should be HD only: %+v", second)
	}
	if len(second.Tags) != 1 || second.Tags[0] != "高清" {
		t.Errorf("second magnet tags mismatch: %v", second.Tags)
	}
}

func TestParseMagnetsHTMLSkipsInvalidHashes(t *testing.T) {
	for _, hash := range []string{"", "abc", "not-a-hash"} {
		body := `<tr><td><a href="magnet:?xt=urn:btih:` + hash + `">SSIS-001</a></td><td>1GB</td><td>2026-09-30</td></tr>`
		magnets, err := parseMagnetsHTML(body)
		if err != nil || len(magnets) != 0 {
			t.Errorf("invalid hash %q: magnets = %+v, error = %v", hash, magnets, err)
		}
	}
}

func TestExtractDetailParams_SSIS001(t *testing.T) {
	content := fixtureFile(t, "detail_ssis-001.html")
	params, err := extractDetailParams(string(content))
	if err != nil {
		t.Fatalf("unexpected error extracting detail params: %v", err)
	}

	if params.GID != "45622804531" {
		t.Errorf("expected GID 45622804531, got %s", params.GID)
	}
	if params.UC != "0" {
		t.Errorf("expected UC 0, got %s", params.UC)
	}
	if params.Img != "/pics/cover/83ie_b.jpg" {
		t.Errorf("expected Img /pics/cover/83ie_b.jpg, got %s", params.Img)
	}
}

func TestNotFoundPage_ZZZZ99999(t *testing.T) {
	content := fixtureFile(t, "notfound_zzzz-99999.html")
	if !isNotFoundPage(string(content)) {
		t.Errorf("expected notfound page to be identified")
	}
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		input string
		want  int64
	}{
		{"2.02GB", 2168958484},
		{"700MB", 734003200},
		{"1.5TB", 1649267441664},
		{"512KB", 524288},
		{"100B", 100},
		{"1PB", 0},
		{"invalid", 0},
		{"", 0},
	}

	for _, c := range cases {
		got := parseSize(c.input)
		if got != c.want {
			t.Errorf("parseSize(%q) = %d, want %d", c.input, got, c.want)
		}
	}
}

func TestCleanDate(t *testing.T) {
	if got := cleanDate("2025-10-28"); got != "2025-10-28" {
		t.Errorf("cleanDate(2025-10-28) = %q, want 2025-10-28", got)
	}
	if got := cleanDate("0000-00-00"); got != "" {
		t.Errorf("cleanDate(0000-00-00) = %q, want empty", got)
	}
	if got := cleanDate("invalid"); got != "" {
		t.Errorf("cleanDate(invalid) = %q, want empty", got)
	}
}

func TestDriverVerifyAndCloudflareCheck(t *testing.T) {
	if !isDriverVerify("<html><a href='/doc/driver-verify'>verify</a></html>") {
		t.Errorf("expected driver-verify detected")
	}
	if !isCloudflareChallenge("<title>Just a moment...</title>") {
		t.Errorf("expected cloudflare detected")
	}
}
