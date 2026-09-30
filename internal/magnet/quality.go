package magnet

import (
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/ppxb/miyabi/internal/domain"
)

var (
	// Ad banners often enclosed in brackets.
	adBracketRegex = regexp.MustCompile(`(?i)[【\[][^】\]]*(?:app|夸克|防失联|免费|地址|网址|发布|域名|代理|论坛|下载)[^】\]]*[】\]]`)

	// Subtitle patterns.
	fhdcRegex    = regexp.MustCompile(`(?i)(?:[^A-Za-z]|^)FHDC(?:[^A-Za-z]|$)`)
	subCodeRegex = regexp.MustCompile(`(?i)[-_](?:UC|CH?)(?:[^A-Za-z]|$)`)

	// 4K patterns.
	fourKRegex = regexp.MustCompile(`(?i)(?:[^A-Za-z0-9]|^)(?:4K(?:UHD)?|2160P)(?:[^A-Za-z0-9]|$)`)

	// Uncensored / leaked patterns.
	uncensoredRegex       = regexp.MustCompile(`(?i)(?:[^A-Za-z0-9]|^)(?:uncensored|mosaic)(?:[^A-Za-z0-9]|$)`)
	uncensoredSuffixRegex = regexp.MustCompile(`(?i)[-_]U(?:C)?(?:[^A-Za-z0-9]|$)`)
	leakedRegex           = regexp.MustCompile(`(?i)(?:[^A-Za-z]|^)leaked(?:[^A-Za-z]|$)`)
	uncensoredWords       = []string{"无码", "無碼"}
	crackedWords          = []string{"破解", "破坏", "破壞", "流出"}

	versionModifierRegex = regexp.MustCompile(`(?i)(破解|破坏|破壞|无码|無碼|流出|高清|版本|原盘|壓制|压制|自制|精修|字幕)`)

	subtitleKeywords = []string{
		"中字", "中文", "字幕", "繁中", "简中", "汉化", "内嵌", "内封", "双语",
	}
)

// QualityInference is what the resource name alone says about a magnet.
type QualityInference struct {
	HasSubtitle   bool
	Has4K         bool
	HasUncensored bool
	HasCracked    bool
}

// CleanTitle strips common advertisement brackets from resource titles.
func CleanTitle(title string) string {
	cleaned := adBracketRegex.ReplaceAllString(title, "")
	return strings.TrimSpace(cleaned)
}

// Infer inspects the resource name and returns inferred quality attributes.
func Infer(name string) QualityInference {
	cleaned := CleanTitle(name)
	res := QualityInference{}

	if fhdcRegex.MatchString(cleaned) || subCodeRegex.MatchString(cleaned) {
		res.HasSubtitle = true
	} else {
		for _, kw := range subtitleKeywords {
			if strings.Contains(cleaned, kw) {
				res.HasSubtitle = true
				break
			}
		}
		if !res.HasSubtitle && hasHanziWithoutKana(cleaned) {
			res.HasSubtitle = true
		}
	}

	if fourKRegex.MatchString(cleaned) {
		res.Has4K = true
	}

	if uncensoredRegex.MatchString(cleaned) || uncensoredSuffixRegex.MatchString(cleaned) {
		res.HasUncensored = true
	} else {
		for _, w := range uncensoredWords {
			if strings.Contains(cleaned, w) {
				res.HasUncensored = true
				break
			}
		}
	}

	if leakedRegex.MatchString(cleaned) {
		res.HasCracked = true
	} else {
		for _, w := range crackedWords {
			if strings.Contains(cleaned, w) {
				res.HasCracked = true
				break
			}
		}
	}

	return res
}

// hasHanziWithoutKana checks if the text contains Chinese Hanzi characters
// (excluding generic version/release modifiers) and zero Japanese kana,
// which strongly indicates Chinese release translated naming.
func hasHanziWithoutKana(s string) bool {
	stripped := versionModifierRegex.ReplaceAllString(s, "")
	hanCount := 0
	for _, r := range stripped {
		if unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			return false
		}
		if unicode.Is(unicode.Han, r) {
			hanCount++
		}
	}
	return hanCount >= 2
}

// ApplyInference adds tags read from the resource name. The site's own
// HasSubtitle and HD flags are never touched, so a 字幕 tag without
// HasSubtitle is known to be inferred, and 4K, 无码 and 破解 are always
// inferred. Inferred is set only when at least one tag came from the name.
// Applying it twice is a no-op.
func ApplyInference(m *domain.Magnet) {
	inf := Infer(m.Name)
	added := false
	add := func(tag string) {
		if !slices.Contains(m.Tags, tag) {
			m.Tags = append(m.Tags, tag)
			added = true
		}
	}
	if inf.HasSubtitle {
		add(domain.MagnetTagSubtitle)
	}
	if inf.Has4K {
		add(domain.MagnetTag4K)
	}
	if inf.HasUncensored {
		add(domain.MagnetTagUncensored)
	}
	if inf.HasCracked {
		add(domain.MagnetTagCracked)
	}
	if added {
		m.Inferred = true
	}
}

func hasTag(m domain.Magnet, tag string) bool {
	return slices.Contains(m.Tags, tag)
}

// HasSubtitle reports a subtitle label from either the site or inference.
func HasSubtitle(m domain.Magnet) bool {
	return m.HasSubtitle || hasTag(m, domain.MagnetTagSubtitle)
}

// IsHD reports an HD label from the site or a 4K marker in the name.
func IsHD(m domain.Magnet) bool {
	return m.HD || hasTag(m, domain.MagnetTag4K)
}

// IsUncensored reports an uncensored or leaked release. Sites never label
// these, so the answer is always inferred.
func IsUncensored(m domain.Magnet) bool {
	return hasTag(m, domain.MagnetTagUncensored) || hasTag(m, domain.MagnetTagCracked)
}
