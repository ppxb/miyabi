package subtitle

import (
	"regexp"
	"strconv"
	"strings"
)

// previewLines bounds the cues returned for a preview.
const previewLines = 20

// Cue is one timed subtitle line; Start is in seconds.
type Cue struct {
	Start float64 `json:"start"`
	Text  string  `json:"text"`
}

// Preview summarizes a downloaded subtitle: its first lines, how many cues it
// has and when the last one ends, next to the movie runtime in minutes, so a
// truncated or mistimed subtitle stands out before it is saved.
type Preview struct {
	Language string  `json:"language"`
	Cues     int     `json:"cues"`
	End      float64 `json:"end"`
	Runtime  int     `json:"runtime"`
	Lines    []Cue   `json:"lines"`
}

var (
	cueTiming = regexp.MustCompile(`^\s*((?:\d{1,2}:)?\d{1,2}:\d{2}[,.]\d{1,3})\s*-->\s*((?:\d{1,2}:)?\d{1,2}:\d{2}[,.]\d{1,3})`)
	assTags   = regexp.MustCompile(`\{[^}]*\}`)
)

func previewOf(text, format string) Preview {
	preview := Preview{Lines: []Cue{}}
	add := func(start, end float64, line string) {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			return
		}
		preview.Cues++
		preview.End = max(preview.End, end)
		if len(preview.Lines) < previewLines {
			preview.Lines = append(preview.Lines, Cue{Start: start, Text: line})
		}
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if format == "ass" || format == "ssa" {
		for _, line := range lines {
			rest, ok := cutPrefixFold(strings.TrimSpace(line), "dialogue:")
			if !ok {
				continue
			}
			// Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
			fields := strings.SplitN(rest, ",", 10)
			if len(fields) < 10 {
				continue
			}
			body := strings.NewReplacer(`\N`, " ", `\n`, " ", `\h`, " ").Replace(assTags.ReplaceAllString(fields[9], ""))
			add(timestamp(fields[1]), timestamp(fields[2]), body)
		}
		return preview
	}
	for i := 0; i < len(lines); i++ {
		match := cueTiming.FindStringSubmatch(lines[i])
		if match == nil {
			continue
		}
		var body []string
		for i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
			i++
			body = append(body, lines[i])
		}
		add(timestamp(match[1]), timestamp(match[2]), strings.Join(body, " "))
	}
	return preview
}

// timestamp parses [h:]m:s[.,]fraction into seconds.
func timestamp(value string) float64 {
	parts := strings.Split(strings.ReplaceAll(strings.TrimSpace(value), ",", "."), ":")
	seconds := 0.0
	for _, part := range parts {
		n, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return 0
		}
		seconds = seconds*60 + n
	}
	return seconds
}

func cutPrefixFold(value, prefix string) (string, bool) {
	if len(value) < len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return "", false
	}
	return value[len(prefix):], true
}
