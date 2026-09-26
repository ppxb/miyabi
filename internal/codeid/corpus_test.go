package codeid

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type catalogueSample struct {
	ID     string `json:"id"`
	Number string `json:"number"`
}

// The fixture is a dated snapshot of the same feed as Miyabi's latest released
// tab. Tests never contact JavDB; filenames below are synthetic, not downloads
// observed on JavDB. Keep provider numbers intact as the independent oracle.
func latestCatalogueSamples(t *testing.T) []catalogueSample {
	t.Helper()
	body, err := os.ReadFile("testdata/javdb_latest.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Movies []catalogueSample `json:"movies"`
	}
	if err := json.Unmarshal(body, &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Movies) != 200 {
		t.Fatalf("corpus has %d movies, want 200", len(snapshot.Movies))
	}
	seen := make(map[string]bool, len(snapshot.Movies))
	for _, sample := range snapshot.Movies {
		if sample.ID == "" || strings.TrimSpace(sample.Number) == "" || seen[sample.ID] {
			t.Fatalf("missing identity or duplicate movie ID: %+v", sample)
		}
		seen[sample.ID] = true
	}
	return snapshot.Movies
}

// Accept presentation differences only. In particular, do not use Normalize,
// Candidates or IsEquivalent to calculate the expected identity: they are the
// behavior being checked, and could silently discard the same suffix as Parse.
func catalogueSpelling(number string) string {
	return strings.ToUpper(strings.ReplaceAll(number, "_", "-"))
}

type catalogueFilename struct {
	name, filename string
}

func catalogueFilenames(code string) []catalogueFilename {
	return []catalogueFilename{
		{"bare", code},
		{"filename", code + ".mp4"},
		{"lowercase", strings.ToLower(code) + ".mkv"},
		{"website_and_codec", "[example.com] " + code + " H.265 1080p.mkv"},
		{"subtitle_marker", code + "-C.mp4"},
		{"disc_marker", code + "-CD1.mkv"},
		{"numeric_part", code + "-02.mp4"},
		{"unicode_separator", strings.NewReplacer("-", "－", "_", "＿").Replace(code) + ".mp4"},
	}
}

func TestJavDBCorpusNormalize(t *testing.T) {
	for _, sample := range latestCatalogueSamples(t) {
		t.Run(sample.ID, func(t *testing.T) {
			got := Normalize(sample.Number)
			if catalogueSpelling(got) != catalogueSpelling(sample.Number) {
				t.Errorf("Normalize(%q) = %q; changed the provider catalogue number", sample.Number, got)
			}
		})
	}
}

func TestJavDBCorpusParse(t *testing.T) {
	for _, sample := range latestCatalogueSamples(t) {
		t.Run(sample.ID, func(t *testing.T) {
			code := sample.Number
			for _, variant := range catalogueFilenames(code) {
				t.Run(variant.name, func(t *testing.T) {
					got, ok := Parse(variant.filename)
					if !ok || catalogueSpelling(got) != catalogueSpelling(code) {
						t.Errorf("Parse(%q) = %q, %t; want complete provider number %q", variant.filename, got, ok, code)
					}
				})
			}
		})
	}
}

func TestJavDBCorpusDistinctIdentities(t *testing.T) {
	samples := latestCatalogueSamples(t)
	for i, first := range samples {
		for _, second := range samples[i+1:] {
			t.Run(first.ID+"_"+second.ID, func(t *testing.T) {
				if IsEquivalent(first.Number, second.Number) {
					t.Errorf("different JavDB IDs %s (%s) and %s (%s) were treated as equivalent",
						first.ID, first.Number, second.ID, second.Number)
				}
			})
		}
	}
}

// These mappings were supplied by the user, not fetched from the latest feed.
// This checks candidate recall only; it does not claim a candidate is enough
// to authorize a movie association without catalogue identity confirmation.
func TestReportedFilenameCandidateRecall(t *testing.T) {
	for _, sample := range []struct {
		filename string
		number   string
	}{
		{"200GANA-3458.mp4", "GANA-3458"},
		{"CARIB-060326-001.mp4", "060326-001"},
	} {
		t.Run(sample.filename, func(t *testing.T) {
			parsed, ok := Parse(sample.filename)
			if !ok {
				t.Fatalf("Parse(%q) found no catalogue candidate", sample.filename)
			}
			candidates := Candidates(parsed)
			for _, candidate := range candidates {
				if catalogueSpelling(candidate) == catalogueSpelling(sample.number) {
					return
				}
			}
			t.Errorf("filename %q parsed as %q; candidates %q omit the confirmed number %q",
				sample.filename, parsed, candidates, sample.number)
		})
	}
}
