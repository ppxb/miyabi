package codeid

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

func random300CatalogueSamples(t *testing.T) []catalogueSample {
	t.Helper()
	body, err := os.ReadFile("testdata/javdb_random_300.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Movies []catalogueSample `json:"movies"`
	}
	if err := json.Unmarshal(body, &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Movies) != 300 {
		t.Fatalf("corpus has %d movies, want 300", len(snapshot.Movies))
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

func randomCatalogueSpelling(number string) string {
	s := strings.ToUpper(number)
	s = strings.Replace(s, "FC2-PPV-", "FC2-", 1)
	return s
}

func TestRandom300Normalize(t *testing.T) {
	for _, sample := range random300CatalogueSamples(t) {
		t.Run(sample.ID, func(t *testing.T) {
			got := Normalize(sample.Number)
			if randomCatalogueSpelling(got) != randomCatalogueSpelling(sample.Number) {
				t.Errorf("Normalize(%q) = %q; changed the provider catalogue number", sample.Number, got)
			}
		})
	}
}

func TestRandom300Parse(t *testing.T) {
	for _, sample := range random300CatalogueSamples(t) {
		t.Run(sample.ID, func(t *testing.T) {
			if sample.ID == "0eE9g7" {
				t.Skip("903-ai format discarded per user decision to prioritize mainstream movies")
			}
			code := sample.Number
			for _, variant := range catalogueFilenames(code) {
				t.Run(variant.name, func(t *testing.T) {
					got, ok := Parse(variant.filename)
					if !ok || randomCatalogueSpelling(got) != randomCatalogueSpelling(code) {
						t.Errorf("Parse(%q) = %q, %t; want complete provider number %q", variant.filename, got, ok, code)
					}
				})
			}
		})
	}
}

func TestRandom300PrototypeFilenames(t *testing.T) {
	samples := random300CatalogueSamples(t)
	pool := slices.Clone(samples)

	for _, reverse := range []bool{false, true} {
		if reverse {
			slices.Reverse(pool)
		}
		t.Run(fmt.Sprintf("reverse_%t", reverse), func(t *testing.T) {
			for _, movie := range samples {
				if movie.ID == "0eE9g7" {
					continue
				}
				for _, variant := range catalogueFilenames(movie.Number) {
					t.Run(movie.ID+"/"+variant.name, func(t *testing.T) {
						id, err := prototypeResolve(variant.filename, func(string) ([]catalogueSample, error) { return pool, nil })
						wantID := movie.ID
						if variant.name == "numeric_part" {
							partCode := Normalize(movie.Number + "-02")
							for _, item := range pool {
								if Normalize(item.Number) == partCode {
									wantID = item.ID
									break
								}
							}
						}
						if err != nil || id != wantID {
							t.Errorf("%q resolved to %q, error = %v; want %s", variant.filename, id, err, wantID)
						}
					})
				}
			}
		})
	}
}
