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
	numberSuffix    = `[0-9]+[A-Z0-9]*(?:[-_. ]+[0-9]+|[-_]+[A-Z0-9]+)*`
	// Western scenes use a site name and a dotted date as the full identifier.
	westernNumber = `(?:[A-Z][A-Z0-9]*|[0-9]+[A-Z][A-Z0-9]*)\.(?:[0-9]{2}|[0-9]{4})\.[0-9]{2}\.[0-9]{2}`
)

var (
	separators          = strings.NewReplacer("－", "-", "﹣", "-", "–", "-", "—", "-", "＿", "_")
	delimiters          = regexp.MustCompile(`[-_. ]+`)
	fc2Pattern          = regexp.MustCompile(`^FC2[-_. ]*(?:PPV)?[-_. ]*([0-9]+)(` + catalogueSuffix + `)$`)
	westernPattern      = regexp.MustCompile(`^(` + westernNumber + `)(` + catalogueSuffix + `)$`)
	numericPattern      = regexp.MustCompile(`^([0-9]{6})[-_]([0-9]{2,3})$`)
	labelPrefix         = regexp.MustCompile(`^[0-9]+[A-Z][A-Z0-9]*$`)
	heydougaPattern     = regexp.MustCompile(`^(HEYDOUGA)[-_. ]*([0-9]{4}[-_. ]+` + numberSuffix + `)$`)
	compactDatePattern  = regexp.MustCompile(`^(` + compactPrefix + `)([0-9]{6}[-_][0-9]{2,3})$`)
	separatedPattern    = regexp.MustCompile(`^(` + prefix + `)[-_. ]+(` + numberSuffix + `)$`)
	letterSerialPattern = regexp.MustCompile(`^(` + prefix + `)[-_ ]+([A-Z]+[0-9][A-Z0-9]*` + catalogueSuffix + `)$`)
	compactPattern      = regexp.MustCompile(`^(` + compactPrefix + `)(` + numberSuffix + `)$`)

	// Heydouga catalogue numbers and six-digit date codes keep their second numeric segment.
	// Other trailing segments in filenames (SSIS-589-02, for example) identify a video part.
	// Never restart inside an unrecognized dash/underscore/dot-delimited token.
	filenamePattern = regexp.MustCompile(`(?:^|[^A-Z0-9_.-])(` +
		westernNumber + `|` +
		fc2Number + `|` +
		heydougaNumber + `|` +
		prefix + `[-_. ]*[0-9]{6}[-_][0-9]{2,3}|` +
		`[0-9]{6}[-_][0-9]{2,3}|` +
		prefix + `[-_ ]+[A-Z]+[0-9][A-Z0-9]*|` +
		prefix + `[-_. ]+[0-9]+[A-Z0-9]*|` +
		compactPrefix + `[0-9]{2,}[A-Z0-9]*)(?:[^A-Z0-9]|$)`)
	domainNoise = regexp.MustCompile(`(?:\[(?:[A-Z0-9-]+\.)+[A-Z]{2,}\]|(?:HTTPS?://)?(?:[A-Z0-9-]+\.)+[A-Z]{2,}[@/\\])`)
	codecNoise  = regexp.MustCompile(`\b[HX][ ._-]?26[45]\b`)
	partNoise   = regexp.MustCompile(`([0-9][A-Z0-9]*)[-_. ]+(?:CD|DISC|PART)[0-9]+([^A-Z0-9]|$)`)
	nameSuffix  = regexp.MustCompile(`^[-_]+([A-Z][A-Z0-9]*)(?:[^A-Z0-9]|$)`)
	fileMarker  = regexp.MustCompile(`^(?:C|U|UC|CHS|CHT|SUB|HD|FHD|UHD|(?:CD|DISC|PART)[0-9]+)$`)
	// SSNI releases append C directly to the numeric serial for subtitles.
	// Limit this filename alias to the known family; other catalogues have real letter variants.
	ssniSubtitle = regexp.MustCompile(`^(SSNI-[0-9]+)C$`)
)

// Parse extracts a catalogue number from a filename, discarding website,
// encoding, subtitle and video-part annotations.
func Parse(name string) (string, bool) {
	name = strings.ToUpper(separators.Replace(name))
	name = domainNoise.ReplaceAllString(name, " ")
	name = codecNoise.ReplaceAllString(name, " ")
	// A compact number followed by CD1 is a file part, not an alphanumeric serial.
	name = partNoise.ReplaceAllString(name, "$1$2")
	match := filenamePattern.FindStringSubmatchIndex(name)
	if match == nil {
		return "", false
	}
	code := Normalize(name[match[2]:match[3]])
	code = ssniSubtitle.ReplaceAllString(code, "$1")
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
	} else if suffix := versionSuffix.FindStringSubmatch(name[match[3]:]); suffix != nil {
		if !fileMarker.MatchString(suffix[1]) {
			code += "-" + suffix[1]
		}
	}
	return code, code != ""
}

var (
	versionSuffix = regexp.MustCompile(`^[-_]([A-Z])(?:[^A-Z0-9]|$)`)
	layerToken    = regexp.MustCompile(`[A-Z0-9]+(?:[-_.][A-Z0-9]+)*`)
	layerSerial   = regexp.MustCompile(`[A-Z][A-Z0-9]*[-_.]?[0-9]`)
	layerNumeric  = regexp.MustCompile(`^[0-9]+[-_][0-9]+(?:[-_][A-Z0-9]+)*$`)
	layerPart     = regexp.MustCompile(`^[0-9]{1,2}$`)
)

// Layers generates catalogue candidates in priority order:
// Level 0: 完整原貌 (Complete normalized catalogue candidate)
// Level 1: 前缀回退 (Prefix stripped: distributor label digits or studio prefix removed)
// Level 2: 后缀回退 (Suffix stripped: video part or subtitle marker removed)
// Level 3: 双端回退 (Both prefix and suffix stripped)
//
// Each candidate is normalized and validated against valid catalogue formats.
func Layers(name string) [][]string {
	name = strings.ToUpper(separators.Replace(name))
	for _, ext := range []string{".MP4", ".MKV", ".NFO", ".STRM"} {
		name = strings.TrimSuffix(name, ext)
	}
	name = domainNoise.ReplaceAllString(name, " ")
	name = codecNoise.ReplaceAllString(name, " ")
	var complete string
	for _, token := range layerToken.FindAllString(name, -1) {
		if layerSerial.MatchString(token) || layerNumeric.MatchString(token) {
			complete = Normalize(token)
			break
		}
	}
	if complete == "" {
		if norm := Normalize(name); norm != "" {
			complete = norm
		} else {
			return nil
		}
	}

	stripPrefix := func(code string) []string {
		var alts []string
		prefix, rest, separated := strings.Cut(code, "-")
		if separated {
			if label := strings.TrimLeft(prefix, "0123456789"); label != "" && label != prefix {
				alts = append(alts, label+"-"+rest)
			}
			if layerNumeric.MatchString(rest) {
				alts = append(alts, rest)
			}
		}
		return alts
	}

	stripSuffix := func(code string) []string {
		var alts []string
		if index := strings.LastIndexByte(code, '-'); index > 0 {
			tail := code[index+1:]
			if fileMarker.MatchString(tail) || layerPart.MatchString(tail) {
				alts = append(alts, code[:index])
			}
		}
		return alts
	}

	valid := func(code string) bool {
		return layerSerial.MatchString(code) || layerNumeric.MatchString(code)
	}

	// Level 0: 完整名称
	layers := [][]string{{complete}}
	seen := map[string]bool{complete: true}

	addLayer := func(candidates []string) {
		var layer []string
		for _, c := range candidates {
			if !seen[c] && valid(c) {
				seen[c] = true
				layer = append(layer, c)
			}
		}
		if len(layer) > 0 {
			layers = append(layers, layer)
		}
	}

	// Level 1: 前缀回退 (先剥离发行商/渠道前缀，保留尾部)
	var prefixFallbacks []string
	for _, code := range layers[0] {
		prefixFallbacks = append(prefixFallbacks, stripPrefix(code)...)
	}
	addLayer(prefixFallbacks)

	// Level 2: 后缀回退 (剥离分卷/字幕后缀)
	var suffixFallbacks []string
	for _, code := range layers[0] {
		suffixFallbacks = append(suffixFallbacks, stripSuffix(code)...)
	}
	addLayer(suffixFallbacks)

	// Level 3: 双端回退 (前缀与后缀均剥离)
	var dualFallbacks []string
	for _, code := range prefixFallbacks {
		dualFallbacks = append(dualFallbacks, stripSuffix(code)...)
	}
	addLayer(dualFallbacks)

	return layers
}

// Queries returns the search query candidates for a catalogue code,
// including its unpadded numeric variant if applicable (e.g. "ABC-00123" -> ["ABC-00123", "ABC-123"]).
func Queries(candidate string) []string {
	queries := []string{candidate}
	if unpadded, ok := UnpaddedNumericCandidate(candidate); ok {
		queries = append(queries, unpadded)
	}
	return queries
}

// Normalize builds a comparison key for a complete catalogue number, applying
// known equivalent spellings and preserving unfamiliar formats. It does not
// validate catalogue syntax, extract filenames, or strip catalogue suffixes.
// Parse alone applies filename conventions; callers must not use Normalize as
// proof that arbitrary text is a catalogue number or a safe filename.
func Normalize(raw string) string {
	value := strings.ToUpper(strings.TrimSpace(separators.Replace(raw)))
	if match := westernPattern.FindStringSubmatch(value); match != nil {
		return match[1] + delimiters.ReplaceAllString(match[2], "-")
	}
	if match := fc2Pattern.FindStringSubmatch(value); match != nil {
		return "FC2-PPV-" + match[1] + delimiters.ReplaceAllString(match[2], "-")
	}
	if match := numericPattern.FindStringSubmatch(value); match != nil {
		sep := "-"
		if strings.Contains(value, "_") {
			sep = "_"
		}
		return match[1] + sep + match[2]
	}
	// Preserve explicit prefixes such as T28 before trying an omitted separator.
	// Known multipart formats also accept compact spellings without losing a numeric segment.
	for _, pattern := range []*regexp.Regexp{heydougaPattern, compactDatePattern, separatedPattern, letterSerialPattern, compactPattern} {
		if match := pattern.FindStringSubmatch(value); match != nil {
			return match[1] + "-" + delimiters.ReplaceAllString(match[2], "-")
		}
	}
	return value
}

// Candidates lists the catalogue numbers that may name the same movie as code,
// most specific first. Release filenames decorate catalogue numbers in ways
// catalogue sites omit: distributor label digits before the prefix (259LUXU-1899
// for LUXU-1899) or a studio before a date code (CARIB-060326-001 for 060326-001).
// Relaxed forms are guesses; callers must confirm them against a catalogue source.
func Candidates(code string) []string {
	norm := Normalize(code)
	if norm == "" {
		return nil
	}
	candidates := []string{norm}
	prefix, seq := splitCode(norm)
	switch {
	case prefix == "":
	case numericPattern.MatchString(seq):
		candidates = append(candidates, seq)
	case labelPrefix.MatchString(prefix):
		candidates = append(candidates, strings.TrimLeft(prefix, "0123456789")+"-"+seq)
	}
	return candidates
}

// IsEquivalent reports whether two catalogue numbers may identify the same movie:
// a candidate of one is format-equivalent to the other (e.g. 200GANA-3458 vs
// GANA-3458, CARIB-060326-001 vs 060326-001, or ABC-00123 vs ABC-123).
func IsEquivalent(a, b string) bool {
	return relaxesTo(a, b) || relaxesTo(b, a)
}

func relaxesTo(code, other string) bool {
	for _, candidate := range Candidates(code) {
		if IsFormatEquivalent(candidate, other) {
			return true
		}
	}
	return false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func splitCode(norm string) (string, string) {
	if numericPattern.MatchString(norm) {
		return "", norm
	}
	if index := strings.IndexByte(norm, '-'); index > 0 {
		return norm[:index], norm[index+1:]
	}
	return "", norm
}

// IsFormatEquivalent reports whether two catalogue numbers identify the same movie
// under format tolerance rules, including delimiter variations and padding zero variations
// in purely numeric sequences (e.g. "ABC00123" vs "ABC-123", "ABP-001" vs "ABP-1", "IPX-052" vs "IPX-52").
// It strictly requires identical catalogue prefixes and rejects letter serial variants
// (e.g. "FJIN-106" vs "FJIN-106A") or partial truncated codes.
func IsFormatEquivalent(a, b string) bool {
	normA := Normalize(a)
	normB := Normalize(b)
	if normA == "" || normB == "" {
		return false
	}
	if normA == normB {
		return true
	}

	prefixA, seqA := splitCode(normA)
	prefixB, seqB := splitCode(normB)

	// Prefixes must be non-empty and strictly identical.
	if prefixA == "" || prefixA != prefixB {
		return false
	}

	// Sequences must both be purely numeric and equal when stripped of leading zeros.
	if isDigits(seqA) && isDigits(seqB) {
		trimmedA := strings.TrimLeft(seqA, "0")
		trimmedB := strings.TrimLeft(seqB, "0")
		if trimmedA == "" {
			trimmedA = "0"
		}
		if trimmedB == "" {
			trimmedB = "0"
		}
		return trimmedA == trimmedB
	}

	return false
}

// UnpaddedNumericCandidate returns a catalogue candidate with leading zeros stripped
// from a purely numeric sequence (e.g. "ABC-00123" -> "ABC-123", "IPX-052" -> "IPX-52").
// It returns false if no padding zeros were present or if the sequence is non-numeric.
func UnpaddedNumericCandidate(raw string) (string, bool) {
	norm := Normalize(raw)
	if norm == "" {
		return "", false
	}
	prefix, seq := splitCode(norm)
	if prefix == "" || seq == "" || !isDigits(seq) {
		return "", false
	}
	if !strings.HasPrefix(seq, "0") {
		return "", false
	}
	trimmed := strings.TrimLeft(seq, "0")
	if trimmed == "" {
		trimmed = "0"
	}
	if trimmed == seq {
		return "", false
	}
	return prefix + "-" + trimmed, true
}

// Prefix extracts the catalogue bucket prefix for a code (e.g. "IPX" from "IPX-123").
// If no prefix is identifiable, it returns "OTHERS".
func Prefix(code string) string {
	norm := Normalize(code)
	prefix, _ := splitCode(norm)
	if prefix != "" {
		return prefix
	}
	return "OTHERS"
}
