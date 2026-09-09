// Package codeid extracts and normalizes JAV catalogue numbers.
package codeid

import (
	"regexp"
	"strings"
)

const (
	prefix         = `(?:[A-Z][A-Z0-9]{0,11}|[0-9]{1,4}[A-Z][A-Z0-9]{0,10})`
	compactPrefix  = `(?:[A-Z]{1,12}|[0-9]{1,4}[A-Z]{1,11})`
	fc2Number      = `FC2[-_. ]*(?:PPV)?[-_. ]*[0-9]{3,8}`
	heydougaNumber = `HEYDOUGA[-_. ]*[0-9]{4}[-_. ]+[0-9]{1,7}`
	numberSuffix   = `[0-9]{2,7}[A-Z]?(?:[-_. ]+[0-9]{1,7})*`
	// Western scenes use a site name and a dotted date as the full identifier.
	westernNumber = `(?:[A-Z][A-Z0-9]*|[0-9]+[A-Z][A-Z0-9]*)\.(?:[0-9]{2}|[0-9]{4})\.[0-9]{2}\.[0-9]{2}`
)

var (
	separators         = strings.NewReplacer("－", "-", "﹣", "-", "–", "-", "—", "-", "＿", "_")
	delimiters         = regexp.MustCompile(`[-_. ]+`)
	fc2Pattern         = regexp.MustCompile(`^FC2[-_. ]*(?:PPV)?[-_. ]*([0-9]{3,8})$`)
	westernPattern     = regexp.MustCompile(`^` + westernNumber + `$`)
	numericPattern     = regexp.MustCompile(`^([0-9]{6})[-_]([0-9]{2,3})$`)
	heydougaPattern    = regexp.MustCompile(`^(HEYDOUGA)[-_. ]*([0-9]{4}(?:[-_. ]+[0-9]{1,7})+)$`)
	compactDatePattern = regexp.MustCompile(`^(` + compactPrefix + `)([0-9]{6}[-_][0-9]{2,3})$`)
	separatedPattern   = regexp.MustCompile(`^(` + prefix + `)[-_. ]+(` + numberSuffix + `)$`)
	compactPattern     = regexp.MustCompile(`^(` + compactPrefix + `)(` + numberSuffix + `)$`)

	// Heydouga catalogue numbers and six-digit date codes keep their second numeric segment.
	// Other trailing segments in filenames (SSIS-589-02, for example) identify a video part.
	filenamePattern = regexp.MustCompile(`(?:^|[^A-Z0-9])(` +
		westernNumber + `|` +
		fc2Number + `|` +
		heydougaNumber + `|` +
		prefix + `[-_. ]*[0-9]{6}[-_][0-9]{2,3}|` +
		`[0-9]{6}[-_][0-9]{2,3}|` +
		prefix + `[-_. ]+[0-9]{2,7}[A-Z]?|` +
		compactPrefix + `[0-9]{2,7}[A-Z]?)(?:[^A-Z0-9]|$)`)
	domainNoise = regexp.MustCompile(`(?:\[(?:[A-Z0-9-]+\.)+[A-Z]{2,}\]|(?:HTTPS?://)?(?:[A-Z0-9-]+\.)+[A-Z]{2,}[@/\\])`)
	codecNoise  = regexp.MustCompile(`\b[HX][ ._-]?26[45]\b`)
)

// Parse extracts a catalogue number from a filename, discarding website,
// encoding, subtitle and video-part annotations.
func Parse(name string) (string, bool) {
	name = strings.ToUpper(separators.Replace(name))
	name = domainNoise.ReplaceAllString(name, " ")
	name = codecNoise.ReplaceAllString(name, " ")
	match := filenamePattern.FindStringSubmatch(name)
	if match == nil {
		return "", false
	}
	code := Normalize(match[1])
	return code, code != ""
}

// Normalize canonicalizes a complete catalogue number. It does not extract a
// number from filenames or discard suffixes; those belong to Parse.
func Normalize(raw string) string {
	value := strings.ToUpper(strings.TrimSpace(separators.Replace(raw)))
	if westernPattern.MatchString(value) {
		return value
	}
	if match := fc2Pattern.FindStringSubmatch(value); match != nil {
		return "FC2-PPV-" + match[1]
	}
	if match := numericPattern.FindStringSubmatch(value); match != nil {
		return match[1] + "-" + match[2]
	}
	// Preserve explicit prefixes such as T28 before trying an omitted separator.
	// Known multipart formats also accept compact spellings without losing a numeric segment.
	for _, pattern := range []*regexp.Regexp{heydougaPattern, compactDatePattern, separatedPattern, compactPattern} {
		if match := pattern.FindStringSubmatch(value); match != nil {
			return match[1] + "-" + delimiters.ReplaceAllString(match[2], "-")
		}
	}
	return ""
}
