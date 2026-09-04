// Package codeid extracts and normalizes JAV catalogue numbers.
package codeid

import (
	"regexp"
	"strings"
)

var (
	// FC2 numbers are commonly written as FC2-PPV-1234567, FC2PPV1234567,
	// or FC2-1234567.  PPV is part of the canonical form for all of them.
	fc2Pattern = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])fc2[-_. ]*(?:ppv)?[-_. ]*([0-9]{3,8})(?:[^a-z0-9]|$)`)

	// A separator is optional in a few release names, but when present it is
	// the most reliable way to distinguish a catalogue number from a title.
	separatedPattern = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])([a-z0-9]{2,12})[-_. ]+([0-9]{2,7})(?:[^a-z0-9]|$)`)

	// Compact numbers such as SSIS001 and 1PONDO123456 have no separator.
	// The two alternatives account for prefixes that start with a letter and
	// prefixes that start with a digit (for example 1PONDO).
	compactPattern = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:([a-z][a-z0-9]{1,10}?)([0-9]{2,7})|([0-9][a-z][a-z0-9]{0,10}?)([0-9]{2,7}))(?:[^a-z0-9]|$)`)
)

// Parse extracts the first catalogue number from name.
//
// The returned number is already normalized.  A false result means that the
// filename does not contain a supported number.
func Parse(name string) (string, bool) {
	name = normalizeSeparators(name)

	if match := fc2Pattern.FindStringSubmatch(name); match != nil {
		return normalizeFC2(match[1]), true
	}
	if match := separatedPattern.FindStringSubmatch(name); match != nil {
		return normalizeParts(match[1], match[2]), true
	}
	if match := compactPattern.FindStringSubmatch(name); match != nil {
		if match[1] != "" {
			return normalizeParts(match[1], match[2]), true
		}
		return normalizeParts(match[3], match[4]), true
	}

	return "", false
}

// Normalize converts a catalogue number to its canonical spelling.
// Invalid or empty input returns an empty string.
func Normalize(raw string) string {
	if code, ok := Parse(raw); ok {
		return code
	}
	return ""
}

func normalizeFC2(number string) string {
	return "FC2-PPV-" + number
}

func normalizeParts(prefix, number string) string {
	return strings.ToUpper(prefix) + "-" + number
}

func normalizeSeparators(value string) string {
	return strings.NewReplacer(
		"－", "-",
		"﹣", "-",
		"–", "-",
		"—", "-",
		"＿", "_",
	).Replace(value)
}
