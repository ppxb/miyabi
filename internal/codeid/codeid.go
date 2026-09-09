// Package codeid extracts and normalizes JAV catalogue numbers.
package codeid

import (
	"regexp"
	"strings"
)

const (
	prefix         = `(?:[A-Z][A-Z0-9]*|[0-9]+[A-Z][A-Z0-9]*)`
	compactPrefix  = `(?:[A-Z]+|[0-9]+[A-Z]+)`
	fc2Number      = `FC2[-_. ]*(?:PPV)?[-_. ]*[0-9]{3,}`
	heydougaNumber = `HEYDOUGA[-_. ]*[0-9]{4}[-_. ]+[0-9]+`
	// Catalogue suffixes can contain names and variants, not just numbers.
	// Words require a dash or underscore so file extensions and titles stay outside the number.
	catalogueSuffix = `(?:[-_]+[A-Z0-9]+)*`
	numberSuffix    = `[0-9]+[A-Z]*(?:[-_. ]+[0-9]+|[-_]+[A-Z0-9]+)*`
	// Western scenes use a site name and a dotted date as the full identifier.
	westernNumber = `(?:[A-Z][A-Z0-9]*|[0-9]+[A-Z][A-Z0-9]*)\.(?:[0-9]{2}|[0-9]{4})\.[0-9]{2}\.[0-9]{2}`
)

var (
	separators         = strings.NewReplacer("－", "-", "﹣", "-", "–", "-", "—", "-", "＿", "_")
	delimiters         = regexp.MustCompile(`[-_. ]+`)
	fc2Pattern         = regexp.MustCompile(`^FC2[-_. ]*(?:PPV)?[-_. ]*([0-9]+)(` + catalogueSuffix + `)$`)
	westernPattern     = regexp.MustCompile(`^(` + westernNumber + `)(` + catalogueSuffix + `)$`)
	numericPattern     = regexp.MustCompile(`^([0-9]{6})[-_]([0-9]{2,3})$`)
	heydougaPattern    = regexp.MustCompile(`^(HEYDOUGA)[-_. ]*([0-9]{4}[-_. ]+` + numberSuffix + `)$`)
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
		prefix + `[-_. ]+[0-9]+[A-Z]*|` +
		compactPrefix + `[0-9]{2,}[A-Z]*)(?:[^A-Z0-9]|$)`)
	domainNoise = regexp.MustCompile(`(?:\[(?:[A-Z0-9-]+\.)+[A-Z]{2,}\]|(?:HTTPS?://)?(?:[A-Z0-9-]+\.)+[A-Z]{2,}[@/\\])`)
	codecNoise  = regexp.MustCompile(`\b[HX][ ._-]?26[45]\b`)
	nameSuffix  = regexp.MustCompile(`^[-_]+([A-Z][A-Z0-9]*)(?:[^A-Z0-9]|$)`)
	fileMarker  = regexp.MustCompile(`^(?:C|U|UC|CHS|CHT|SUB|HD|FHD|UHD|(?:CD|DISC|PART)[0-9]+)$`)
)

// Parse extracts a catalogue number from a filename, discarding website,
// encoding, subtitle and video-part annotations.
func Parse(name string) (string, bool) {
	name = strings.ToUpper(separators.Replace(name))
	name = domainNoise.ReplaceAllString(name, " ")
	name = codecNoise.ReplaceAllString(name, " ")
	match := filenamePattern.FindStringSubmatchIndex(name)
	if match == nil {
		return "", false
	}
	code := Normalize(name[match[2]:match[3]])
	// SCUTE includes the model name in its catalogue number. Other studios'
	// filename titles must not become catalogue suffixes just because they contain dashes.
	if strings.HasPrefix(code, "SCUTE-") {
		tail := name[match[3]:]
		for {
			suffix := nameSuffix.FindStringSubmatchIndex(tail)
			if suffix == nil {
				break
			}
			part := tail[suffix[2]:suffix[3]]
			if fileMarker.MatchString(part) {
				break
			}
			code += "-" + part
			tail = tail[suffix[3]:]
		}
	}
	return code, code != ""
}

// Normalize canonicalizes a complete catalogue number. It does not extract a
// number from filenames or guess whether a suffix is a video-part or subtitle
// marker; only Parse applies filename conventions.
func Normalize(raw string) string {
	value := strings.ToUpper(strings.TrimSpace(separators.Replace(raw)))
	if match := westernPattern.FindStringSubmatch(value); match != nil {
		return match[1] + delimiters.ReplaceAllString(match[2], "-")
	}
	if match := fc2Pattern.FindStringSubmatch(value); match != nil {
		return "FC2-PPV-" + match[1] + delimiters.ReplaceAllString(match[2], "-")
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
