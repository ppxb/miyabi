// Package sukebei provides a magnet source backed by the public Sukebei
// torrent index (sukebei.nyaa.si). It searches by catalogue code over RSS and
// returns the info hashes, never downloading or seeding anything.
package sukebei

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// feedEntry is one parsed torrent row from the Sukebei RSS feed.
type feedEntry struct {
	Title     string
	InfoHash  string
	Size      int64
	CreatedAt string
}

type rssFeed struct {
	Channel struct {
		Items []struct {
			Title    string `xml:"title"`
			PubDate  string `xml:"pubDate"`
			InfoHash string `xml:"infoHash"`
			Size     string `xml:"size"`
		} `xml:"item"`
	} `xml:"channel"`
}

// parseFeed decodes the Sukebei RSS body into torrent entries. Rows without a
// valid 40-character info hash are dropped.
func parseFeed(body []byte) ([]feedEntry, error) {
	var doc rssFeed
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse Sukebei RSS: %w", err)
	}
	entries := make([]feedEntry, 0, len(doc.Channel.Items))
	for _, item := range doc.Channel.Items {
		hash := strings.ToLower(strings.TrimSpace(item.InfoHash))
		if !isHex40(hash) {
			continue
		}
		entries = append(entries, feedEntry{
			Title:     strings.TrimSpace(item.Title),
			InfoHash:  hash,
			Size:      parseSize(item.Size),
			CreatedAt: parseDate(item.PubDate),
		})
	}
	return entries, nil
}

func isHex40(value string) bool {
	if len(value) != 40 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

var sizeUnits = map[string]int64{
	"B":   1,
	"KIB": 1024,
	"MIB": 1024 * 1024,
	"GIB": 1024 * 1024 * 1024,
	"TIB": 1024 * 1024 * 1024 * 1024,
}

// parseSize parses a Sukebei size such as "1.5 GiB" into bytes.
func parseSize(raw string) int64 {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) != 2 {
		return 0
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || value <= 0 {
		return 0
	}
	multiplier, ok := sizeUnits[strings.ToUpper(fields[1])]
	if !ok {
		return 0
	}
	return int64(value * float64(multiplier))
}

// compilationMarkers flag multi-work collections that must not be auto-picked for
// a single-title subscription: a "BEST of" or "總集編" pack would download a whole
// library instead of one film.
var compilationMarkers = regexp.MustCompile(`(?i)合集|総集編|总集编|总集篇|精選|精选|\b(?:best|complete|collection)\b`)

// isCompilation reports whether a torrent title looks like a compilation pack.
func isCompilation(title string) bool {
	return compilationMarkers.MatchString(title)
}

// parseDate normalizes an RSS pubDate to YYYY-MM-DD in UTC; unknown formats
// yield an empty string so the aggregator keeps its own ordering.
func parseDate(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339} {
		if moment, err := time.Parse(layout, value); err == nil {
			return moment.UTC().Format("2006-01-02")
		}
	}
	return ""
}
