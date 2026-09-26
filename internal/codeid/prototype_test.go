package codeid

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// This experiment is deliberately test-only. Scan and import still use Parse;
// changing their identity/merge semantics requires a separate integration step.
var (
	prototypeToken   = regexp.MustCompile(`[A-Z0-9]+(?:[-_.][A-Z0-9]+)*`)
	prototypeSerial  = regexp.MustCompile(`[A-Z][A-Z0-9]*[-_.]?[0-9]`)
	prototypeNumeric = regexp.MustCompile(`^[0-9]+[-_][0-9]+(?:[-_][A-Z0-9]+)*$`)
	prototypePart    = regexp.MustCompile(`^[0-9]{1,2}$`)
)

func prototypeLayers(filename string) [][]string {
	name := strings.ToUpper(separators.Replace(filename))
	for _, ext := range []string{".MP4", ".MKV", ".NFO", ".STRM"} {
		name = strings.TrimSuffix(name, ext)
	}
	name = domainNoise.ReplaceAllString(name, " ")
	name = codecNoise.ReplaceAllString(name, " ")
	var complete string
	for _, token := range prototypeToken.FindAllString(name, -1) {
		if prototypeSerial.MatchString(token) || prototypeNumeric.MatchString(token) {
			complete = Normalize(token)
			break
		}
	}
	if complete == "" {
		return nil
	}
	layers := [][]string{{complete}}
	seen := map[string]bool{complete: true}
	for level := 0; level < len(layers); level++ {
		var next []string
		for _, code := range layers[level] {
			var alternatives []string
			prefix, rest, separated := strings.Cut(code, "-")
			if separated {
				if label := strings.TrimLeft(prefix, "0123456789"); label != "" && label != prefix {
					alternatives = append(alternatives, label+"-"+rest)
				}
				if prototypeNumeric.MatchString(rest) {
					alternatives = append(alternatives, rest)
				}
			}
			if index := strings.LastIndexByte(code, '-'); index > 0 {
				tail := code[index+1:]
				// Short numeric tails are only a filename-part hypothesis. The
				// complete code always gets a chance to resolve before removal.
				if fileMarker.MatchString(tail) || prototypePart.MatchString(tail) {
					alternatives = append(alternatives, code[:index])
				}
			}
			for _, alternative := range alternatives {
				if !seen[alternative] && (prototypeSerial.MatchString(alternative) || prototypeNumeric.MatchString(alternative)) {
					seen[alternative] = true
					next = append(next, alternative)
				}
			}
		}
		if len(next) > 0 {
			layers = append(layers, next)
		}
	}
	return layers
}

// Query spellings change retrieval only; every returned number is still
// compared with the original candidate, keeping multipart identities intact.
func prototypeQueries(candidate string) []string {
	queries := []string{candidate}
	if unpadded, ok := UnpaddedNumericCandidate(candidate); ok {
		queries = append(queries, unpadded)
	}
	for _, separator := range []string{"", " "} {
		query := strings.NewReplacer("-", separator, "_", separator, ".", separator).Replace(candidate)
		if !slices.Contains(queries, query) {
			queries = append(queries, query)
		}
	}
	return queries
}

func prototypeMatch(candidates []string, movies []catalogueSample, equivalent bool) (string, error) {
	ids := make(map[string]bool)
	for _, movie := range movies {
		for _, candidate := range candidates {
			matches := Normalize(movie.Number) == candidate
			if equivalent {
				matches = IsFormatEquivalent(movie.Number, candidate)
			}
			if matches {
				ids[movie.ID] = true
			}
		}
	}
	if len(ids) > 1 {
		return "", fmt.Errorf("ambiguous catalogue candidates: %v", candidates)
	}
	for id := range ids {
		return id, nil
	}
	return "", nil
}

func prototypeResolve(filename string, search func(string) ([]catalogueSample, error)) (string, error) {
	layers := prototypeLayers(filename)
	var movies []catalogueSample
	queried := make(map[string]bool)
	for level, candidates := range layers {
		for _, candidate := range candidates {
			for _, query := range prototypeQueries(candidate) {
				id, err := prototypeMatch([]string{candidate}, movies, false)
				if err != nil {
					return "", err
				}
				if id != "" {
					break
				}
				if queried[query] {
					continue
				}
				queried[query] = true
				results, err := search(query)
				if err != nil {
					return "", err
				}
				movies = append(movies, results...)
			}
		}
		// A weaker query may reveal a stronger match omitted by earlier
		// searches. Always recheck completed levels in priority order.
		for _, completed := range layers[:level+1] {
			for _, equivalent := range []bool{false, true} {
				id, err := prototypeMatch(completed, movies, equivalent)
				if id != "" || err != nil {
					return id, err
				}
			}
		}
	}
	return "", fmt.Errorf("no catalogue match for %q", filename)
}

func catalogueSearchSamples(t *testing.T) map[string][]catalogueSample {
	t.Helper()
	body, err := os.ReadFile("testdata/javdb_search.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Responses []struct {
			Query  string            `json:"q"`
			Movies []catalogueSample `json:"movies"`
		} `json:"responses"`
	}
	if err := json.Unmarshal(body, &snapshot); err != nil {
		t.Fatal(err)
	}
	responses := make(map[string][]catalogueSample)
	for _, response := range snapshot.Responses {
		if _, duplicate := responses[response.Query]; duplicate || response.Query == "" {
			t.Fatalf("invalid or repeated query %q", response.Query)
		}
		responses[response.Query] = response.Movies
	}
	return responses
}

func TestPrototypeLatestLiveSearch(t *testing.T) {
	responses := catalogueSearchSamples(t)
	for _, movie := range latestCatalogueSamples(t) {
		t.Run(movie.ID, func(t *testing.T) {
			id, err := prototypeResolve(movie.Number+".mp4", func(query string) ([]catalogueSample, error) {
				movies, captured := responses[query]
				if !captured {
					return nil, fmt.Errorf("uncaptured query %q", query)
				}
				return movies, nil
			})
			if err != nil || id != movie.ID {
				t.Errorf("%s resolved to %q, error = %v; want JavDB ID %s", movie.Number, id, err, movie.ID)
			}
		})
	}
}

func TestPrototypeLatestFilenames(t *testing.T) {
	samples := latestCatalogueSamples(t)
	// This is a closed catalogue experiment, NOT a live search simulation.
	// Pool real neighbours as distractors, then supply all records for every
	// query. Only the separate live replay tests measure search recall.
	pool := slices.Clone(samples)
	for _, movies := range catalogueSearchSamples(t) {
		pool = append(pool, movies...)
	}
	for _, reverse := range []bool{false, true} {
		if reverse {
			slices.Reverse(pool)
		}
		t.Run(fmt.Sprintf("reverse_%t", reverse), func(t *testing.T) {
			for _, movie := range samples {
				for _, variant := range catalogueFilenames(movie.Number) {
					t.Run(movie.ID+"/"+variant.name, func(t *testing.T) {
						id, err := prototypeResolve(variant.filename, func(string) ([]catalogueSample, error) { return pool, nil })
						if err != nil || id != movie.ID {
							t.Errorf("%q resolved to %q, error = %v; want %s", variant.filename, id, err, movie.ID)
						}
					})
				}
			}
		})
	}
}

func TestPrototypeResolutionPriority(t *testing.T) {
	for _, test := range []struct {
		name, input, want, errorText string
		responses                    map[string][]catalogueSample
		queries                      []string
	}{
		{
			name: "complete beats stripped prefix", input: "200GANA-3458.mp4", want: "complete",
			responses: map[string][]catalogueSample{"200GANA-3458": {{"relaxed", "GANA-3458"}, {"complete", "200GANA-3458"}}},
			queries:   []string{"200GANA-3458"},
		},
		{
			name: "complete formatting queries finish before prefix removal", input: "326IHD-005.mp4", want: "complete",
			responses: map[string][]catalogueSample{
				"326IHD-005": {{"relaxed", "IHD-005"}},
				"326IHD-5":   {{"complete", "326IHD-005"}},
			},
			queries: []string{"326IHD-005", "326IHD-5"},
		},
		{
			name: "reuse relaxed match after complete queries finish", input: "200GANA-3458.mp4", want: "relaxed",
			responses: map[string][]catalogueSample{
				"200GANA-3458": {{"relaxed", "GANA-3458"}}, "200GANA3458": {}, "200GANA 3458": {},
			},
			queries: []string{"200GANA-3458", "200GANA3458", "200GANA 3458"},
		},
		{
			name: "version remains distinct", input: "START-637-V.mp4", want: "variant",
			responses: map[string][]catalogueSample{"START-637-V": {{"base", "START-637"}, {"variant", "START-637-V"}}},
			queries:   []string{"START-637-V"},
		},
		{
			name: "missing version is unresolved", input: "START-637-V.mp4", errorText: "no catalogue match",
			responses: map[string][]catalogueSample{
				"START-637-V": {{"base", "START-637"}}, "START637V": {}, "START 637 V": {},
			},
			queries: []string{"START-637-V", "START637V", "START 637 V"},
		},
		{
			name: "multiple exact IDs stop resolution", input: "200GANA-3458.mp4", errorText: "ambiguous",
			responses: map[string][]catalogueSample{"200GANA-3458": {{"first", "200GANA-3458"}, {"second", "200GANA-3458"}, {"relaxed", "GANA-3458"}}},
			queries:   []string{"200GANA-3458"},
		},
		{
			name: "same-level candidates cannot pick different movies", input: "200ABC-123456-789.mp4", errorText: "ambiguous",
			responses: map[string][]catalogueSample{
				"200ABC-123456-789": {}, "200ABC123456789": {}, "200ABC 123456 789": {},
				"ABC-123456-789": {{"named", "ABC-123456-789"}},
				"123456-789":     {{"numeric", "123456-789"}},
			},
			queries: []string{"200ABC-123456-789", "200ABC123456789", "200ABC 123456 789", "ABC-123456-789", "123456-789"},
		},
		{
			name: "duplicate rows share one identity", input: "GANA-3458.mp4", want: "same",
			responses: map[string][]catalogueSample{"GANA-3458": {{"same", "GANA-3458"}, {"same", "gana3458"}}},
			queries:   []string{"GANA-3458"},
		},
		{
			name: "nearby numbers do not match", input: "GANA-3458.mp4", errorText: "no catalogue match",
			responses: map[string][]catalogueSample{
				"GANA-3458": {{"nearby", "GANA-3459"}, {"longer", "GANA-34580"}}, "GANA3458": {}, "GANA 3458": {},
			},
			queries: []string{"GANA-3458", "GANA3458", "GANA 3458"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var queries []string
			id, err := prototypeResolve(test.input, func(query string) ([]catalogueSample, error) {
				queries = append(queries, query)
				movies, ok := test.responses[query]
				if !ok {
					return nil, fmt.Errorf("unexpected query %q", query)
				}
				return movies, nil
			})
			if id != test.want || (test.errorText == "" && err != nil) || (test.errorText != "" && (err == nil || !strings.Contains(err.Error(), test.errorText))) {
				t.Fatalf("got %q, %v; want %q, error containing %q", id, err, test.want, test.errorText)
			}
			if !slices.Equal(queries, test.queries) {
				t.Fatalf("queries = %v, want %v", queries, test.queries)
			}
		})
	}
	t.Run("network failure is not a reason to relax identity", func(t *testing.T) {
		networkErr := errors.New("connection failed")
		calls := 0
		id, err := prototypeResolve("200GANA-3458.mp4", func(string) ([]catalogueSample, error) { calls++; return nil, networkErr })
		if id != "" || !errors.Is(err, networkErr) || calls != 1 {
			t.Fatalf("got %q, %v after %d calls", id, err, calls)
		}
	})
}
