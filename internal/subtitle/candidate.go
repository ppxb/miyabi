package subtitle

import (
	"cmp"
	"slices"

	"github.com/ppxb/miyabi/internal/codeid"
)

// Candidate is a subtitle offered by an online provider.
type Candidate struct {
	Provider string
	Name     string
	URL      string
	Format   string
	// Language is LangUnknown when the provider gave no hint; it is then
	// detected from the downloaded text.
	Language Language
	Version  VersionTag
	Score    int
}

// Kind groups interchangeable subtitles. Emby lists one track per kind, so a
// movie keeps at most one subtitle of each.
type Kind struct {
	Language Language
	Version  VersionTag
	Format   string
}

func (c Candidate) Kind() Kind {
	return Kind{Language: c.Language, Version: c.Version, Format: c.Format}
}

// Rank keeps candidates that name the movie's catalogue number and orders
// them by relevance. Provider search is fuzzy: querying ABP-1 also returns
// ABP-123, so the catalogue number parsed from each name must be equivalent
// to code (padding, label digits and delimiters may differ).
func Rank(candidates []Candidate, code string, uncensored bool) []Candidate {
	ranked := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !identifies(candidate.Name, code) || candidate.Format == "" {
			continue
		}
		// Uncensored cuts are retimed; their subtitles drift on censored releases.
		leaked := candidate.Version == VersionUncensored || candidate.Version == VersionLeaked
		if leaked && !uncensored {
			continue
		}
		candidate.Score = score(candidate, uncensored)
		ranked = append(ranked, candidate)
	}
	slices.SortStableFunc(ranked, func(a, b Candidate) int { return cmp.Compare(b.Score, a.Score) })
	return ranked
}

func identifies(name, code string) bool {
	parsed, ok := codeid.Parse(name)
	return ok && codeid.IsEquivalent(parsed, code)
}

func score(candidate Candidate, uncensored bool) int {
	score := 0
	switch candidate.Language {
	case LangSimplifiedChinese:
		score += 150
	case LangTraditionalChinese:
		score += 120
	default:
		score += 100
	}
	// SRT renders on every Emby client; ASS keeps styling but may be burned in by the server.
	switch candidate.Format {
	case "srt":
		score += 30
	case "ass", "ssa":
		score += 20
	default:
		score += 10
	}
	switch {
	case uncensored && (candidate.Version == VersionUncensored || candidate.Version == VersionLeaked):
		score += 150
	case candidate.Version == VersionStandard:
		score += 60
	}
	return score
}
