package magnet

import (
	"cmp"
	"slices"

	"github.com/ppxb/miyabi/internal/domain"
)

// PreferenceLevel specifies how much an attribute such as subtitles matters.
type PreferenceLevel string

const (
	PreferencePreferred PreferenceLevel = "preferred" // 优先
	PreferenceRequired  PreferenceLevel = "required"  // 必须
	PreferenceAny       PreferenceLevel = "any"       // 不限
)

// UncensoredFilter specifies tolerance for uncensored / leaked releases.
type UncensoredFilter string

const (
	UncensoredPreferred UncensoredFilter = "preferred" // 优先
	UncensoredRequired  UncensoredFilter = "required"  // 必须
	UncensoredExclude   UncensoredFilter = "exclude"   // 排除
	UncensoredAny       UncensoredFilter = "any"       // 不限
)

// Preferences configures magnet selection.
type Preferences struct {
	Subtitle   PreferenceLevel  `json:"subtitle"`
	HD         PreferenceLevel  `json:"hd"`
	Uncensored UncensoredFilter `json:"uncensored"`
}

// DefaultPreferences prefers subtitles and HD and does not care about censorship.
func DefaultPreferences() Preferences {
	return Preferences{Subtitle: PreferencePreferred, HD: PreferencePreferred, Uncensored: UncensoredAny}
}

// Normalized fills empty fields with the defaults.
func (p Preferences) Normalized() Preferences {
	defaults := DefaultPreferences()
	if p.Subtitle == "" {
		p.Subtitle = defaults.Subtitle
	}
	if p.HD == "" {
		p.HD = defaults.HD
	}
	if p.Uncensored == "" {
		p.Uncensored = defaults.Uncensored
	}
	return p
}

// Validate rejects values outside the enumerations.
func (p Preferences) Validate() error {
	switch p.Subtitle {
	case PreferencePreferred, PreferenceRequired, PreferenceAny:
	default:
		return domain.E(domain.KindInvalid, "字幕偏好无效", nil)
	}
	switch p.HD {
	case PreferencePreferred, PreferenceRequired, PreferenceAny:
	default:
		return domain.E(domain.KindInvalid, "高清偏好无效", nil)
	}
	switch p.Uncensored {
	case UncensoredPreferred, UncensoredRequired, UncensoredExclude, UncensoredAny:
	default:
		return domain.E(domain.KindInvalid, "无码偏好无效", nil)
	}
	return nil
}

// Picker selects the best magnet for a set of preferences.
type Picker struct {
	prefs Preferences
}

func NewPicker(prefs Preferences) *Picker {
	return &Picker{prefs: prefs.Normalized()}
}

// Score tiers keep the attributes strictly ordered: subtitle beats HD beats
// uncensored. An inferred attribute is worth half of a site-verified one.
const (
	scoreSubtitle   = 10000
	scoreHD         = 1000
	scoreUncensored = 100
)

type candidate struct {
	magnet domain.Magnet
	score  int
}

// Pick filters by the required and excluded preferences, scores the rest by
// the preferred ones, and returns the best magnet. The second result is false
// when nothing qualifies. Input magnets must already include aggregated inference tags.
func (p *Picker) Pick(magnets []domain.Magnet) (domain.Magnet, bool) {
	var passed []candidate
	for _, m := range magnets {
		verifiedSubtitle := m.HasSubtitle
		inferredSubtitle := !verifiedSubtitle && hasTag(m, domain.MagnetTagSubtitle)
		verifiedHD := m.HD
		inferredHD := !verifiedHD && hasTag(m, domain.MagnetTag4K)
		uncensored := IsUncensored(m)

		if p.prefs.Subtitle == PreferenceRequired && !verifiedSubtitle && !inferredSubtitle {
			continue
		}
		if p.prefs.HD == PreferenceRequired && !verifiedHD && !inferredHD {
			continue
		}
		if p.prefs.Uncensored == UncensoredExclude && uncensored {
			continue
		}
		if p.prefs.Uncensored == UncensoredRequired && !uncensored {
			continue
		}

		score := 0
		if p.prefs.Subtitle == PreferencePreferred {
			score += weighted(scoreSubtitle, verifiedSubtitle, inferredSubtitle)
		}
		if p.prefs.HD == PreferencePreferred {
			score += weighted(scoreHD, verifiedHD, inferredHD)
		}
		if p.prefs.Uncensored == UncensoredPreferred && uncensored {
			score += scoreUncensored / 2
		}
		passed = append(passed, candidate{magnet: m, score: score})
	}
	if len(passed) == 0 {
		return domain.Magnet{}, false
	}

	// Ties: larger, more files, JavDB-listed, then most recently added.
	slices.SortStableFunc(passed, func(a, b candidate) int {
		if a.score != b.score {
			return cmp.Compare(b.score, a.score)
		}
		return compareMagnets(a.magnet, b.magnet)
	})
	return passed[0].magnet, true
}

func weighted(full int, verified, inferred bool) int {
	switch {
	case verified:
		return full
	case inferred:
		return full / 2
	default:
		return 0
	}
}

// compareMagnets orders by size, file count, JavDB presence and newest date.
func compareMagnets(a, b domain.Magnet) int {
	if a.Size != b.Size {
		return cmp.Compare(b.Size, a.Size)
	}
	if a.FilesCount != b.FilesCount {
		return cmp.Compare(b.FilesCount, a.FilesCount)
	}
	aJavDB := slices.Contains(a.Sources, domain.MagnetSourceJavDB)
	bJavDB := slices.Contains(b.Sources, domain.MagnetSourceJavDB)
	if aJavDB != bJavDB {
		if aJavDB {
			return -1
		}
		return 1
	}
	return cmp.Compare(b.CreatedAt, a.CreatedAt)
}
