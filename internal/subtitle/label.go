package subtitle

import (
	"regexp"
	"strings"
)

// Language identifies the Chinese variant of a subtitle. Values are the
// culture codes Emby reads from subtitle file names.
type Language string

const (
	LangSimplifiedChinese  Language = "zh-CN"
	LangTraditionalChinese Language = "zh-TW"
	LangUnknown            Language = ""
)

// VersionTag identifies the release cut a subtitle was timed against.
type VersionTag string

const (
	VersionStandard   VersionTag = "standard"
	VersionUncensored VersionTag = "uncensored"
	VersionExtended   VersionTag = "extended"
	VersionLeaked     VersionTag = "leaked"
)

var (
	// U and UC suffixes mark uncensored releases, as in SSIS-589-UC.mp4.
	uncensoredPattern = regexp.MustCompile(`(?i)(uncensored|无码|無碼|破解|流出|mosaic|步兵|[-_]uc?(?:[-_. ]|$))`)
	leakedPattern     = regexp.MustCompile(`(?i)(流出|leaked)`)
	extendedPattern   = regexp.MustCompile(`(?i)(extended|加长|加長|完整版|完全版)`)
	// C, UC and CH suffixes mark releases with burned-in Chinese subtitles.
	hardSubtitlePattern = regexp.MustCompile(`(?i)([-_](?:u?c|ch)(?:[-_. ]|$)|中字|中文字幕)`)

	traditionalHint = regexp.MustCompile(`(?i)(zh[-_]?(tw|hk|hant)|\bcht\b|繁)`)
	simplifiedHint  = regexp.MustCompile(`(?i)(zh[-_]?(cn|hans)|\bchs\b|简|簡)`)

	// Frequency counters for discriminating Simplified vs Traditional Chinese.
	simplifiedMarkers  = []rune("为与个么开关东风头后发这边过还进时样气应实话说问题现经体万台农国门书车云")
	traditionalMarkers = []rune("為與個麼開關東風頭後發這邊過還進時樣氣應實話說問題現經體萬臺農國門書車雲")
)

// DetectVersion inspects a filename or metadata title for cut/version markers.
func DetectVersion(name string) VersionTag {
	if leakedPattern.MatchString(name) {
		return VersionLeaked
	}
	if uncensoredPattern.MatchString(name) {
		return VersionUncensored
	}
	if extendedPattern.MatchString(name) {
		return VersionExtended
	}
	return VersionStandard
}

// IsUncensored reports whether a video or subtitle name indicates an uncensored or leaked release.
func IsUncensored(name string) bool {
	version := DetectVersion(name)
	return version == VersionUncensored || version == VersionLeaked
}

// HasHardSubtitle reports whether a video name marks burned-in Chinese subtitles.
func HasHardSubtitle(name string) bool {
	return hardSubtitlePattern.MatchString(name)
}

// LanguageHint recognizes an explicit Chinese variant in a file name or
// provider metadata, returning LangUnknown when there is none.
func LanguageHint(hint string) Language {
	switch {
	case traditionalHint.MatchString(hint):
		return LangTraditionalChinese
	case simplifiedHint.MatchString(hint):
		return LangSimplifiedChinese
	default:
		return LangUnknown
	}
}

// DetectLanguage prefers an explicit hint, then counts characters that differ
// between Simplified and Traditional Chinese. It defaults to Simplified.
func DetectLanguage(hint, text string) Language {
	if language := LanguageHint(hint); language != LangUnknown {
		return language
	}
	// The first 15000 characters are enough to tell the variants apart.
	runes := []rune(text)
	if len(runes) > 15000 {
		runes = runes[:15000]
	}
	var simplified, traditional int
	for _, r := range runes {
		if containsRune(simplifiedMarkers, r) {
			simplified++
		} else if containsRune(traditionalMarkers, r) {
			traditional++
		}
	}
	if traditional > simplified {
		return LangTraditionalChinese
	}
	return LangSimplifiedChinese
}

func containsRune(set []rune, r rune) bool {
	for _, candidate := range set {
		if candidate == r {
			return true
		}
	}
	return false
}

// Format returns the normalized subtitle format of a file extension, or ""
// when Emby cannot load it as an external text subtitle.
func Format(ext string) string {
	switch format := strings.ToLower(strings.TrimPrefix(ext, ".")); format {
	case "srt", "ass", "ssa", "vtt":
		return format
	default:
		return ""
	}
}
