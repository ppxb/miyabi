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

// TimedCue is one subtitle cue in seconds, shaped as the player's text cue.
// Text keeps the cue's line breaks.
type TimedCue struct {
	StartTime float64 `json:"startTime"`
	EndTime   float64 `json:"endTime"`
	Text      string  `json:"text"`
}

func previewOf(text, format string) Preview {
	preview := Preview{Lines: []Cue{}}
	for _, cue := range parseCues(text, format) {
		preview.Cues++
		preview.End = max(preview.End, cue.EndTime)
		if len(preview.Lines) < previewLines {
			preview.Lines = append(preview.Lines, Cue{Start: cue.StartTime, Text: strings.Join(strings.Fields(cue.Text), " ")})
		}
	}
	return preview
}

// parseCues reads SRT/VTT timing blocks or ASS/SSA Dialogue lines, dropping
// ASS override tags and cues without text.
func parseCues(text, format string) []TimedCue {
	cues := []TimedCue{}
	add := func(start, end, body string) {
		if body = strings.TrimSpace(body); body != "" {
			cues = append(cues, TimedCue{StartTime: timestamp(start), EndTime: timestamp(end), Text: body})
		}
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if format == "ass" || format == "ssa" {
		breaks := strings.NewReplacer(`\N`, "\n", `\n`, "\n", `\h`, " ")
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
			add(fields[1], fields[2], breaks.Replace(assTags.ReplaceAllString(fields[9], "")))
		}
		return cues
	}
	for i := 0; i < len(lines); i++ {
		match := cueTiming.FindStringSubmatch(lines[i])
		if match == nil {
			continue
		}
		var body []string
		for i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
			i++
			body = append(body, strings.TrimSpace(lines[i]))
		}
		add(match[1], match[2], strings.Join(body, "\n"))
	}
	return cues
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
