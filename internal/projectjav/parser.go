package projectjav

import (
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
)

const cardMarker = `<div class="col-md-4 mb-4 video-item"`

var (
	movieHrefRe  = regexp.MustCompile(`(?i)href="/movie/([^"/]+)"`)
	slugCodeRe   = regexp.MustCompile(`^(.*)-\d+$`)
	rowRe        = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	magnetHrefRe = regexp.MustCompile(`(?i)href="(magnet:\?xt=urn:btih:[^"]+)"`)
	hashRe       = regexp.MustCompile(`(?i)urn:btih:([0-9a-f]{40})`)
	sizeRe       = regexp.MustCompile(`(?i)fa-weight-hanging"></i>\s*([0-9.]+)\s*(tb|gb|mb|kb)`)
	dateRe       = regexp.MustCompile(`(?i)fa-calendar-alt"></i>\s*(\d{2})/(\d{2})/(\d{2})`)
)

// parseSearch extracts the magnets of the movie whose code matches code from a
// ProjectJav search-results page. Every result card carries the movie's magnet
// rows inline; cards for other codes are ignored.
func parseSearch(body []byte, code string) []domain.Magnet {
	source := string(body)
	want := codeid.Normalize(code)
	var result []domain.Magnet
	seen := make(map[string]bool)
	for _, card := range splitCards(source) {
		href := movieHrefRe.FindStringSubmatch(card)
		if href == nil || !cardMatches(href[1], want) {
			continue
		}
		for _, row := range rowRe.FindAllStringSubmatch(card, -1) {
			cell := row[1]
			if !strings.Contains(strings.ToLower(cell), "magnet:?xt=urn:btih") {
				continue
			}
			magnetHref := magnetHrefRe.FindStringSubmatch(cell)
			if magnetHref == nil {
				continue
			}
			hash := hashRe.FindStringSubmatch(magnetHref[1])
			if hash == nil {
				continue
			}
			infoHash := strings.ToLower(hash[1])
			if seen[infoHash] {
				continue
			}
			seen[infoHash] = true
			result = append(result, domain.Magnet{
				Hash:      infoHash,
				Name:      decodeDn(magnetHref[1]),
				Size:      parseSize(cell),
				CreatedAt: parseDate(cell),
				Sources:   []string{domain.MagnetSourceProjectJav},
			})
		}
	}
	return result
}

// splitCards cuts the document into per-movie card blocks.
func splitCards(source string) []string {
	lower := strings.ToLower(source)
	marker := strings.ToLower(cardMarker)
	var starts []int
	for from := 0; ; {
		index := strings.Index(lower[from:], marker)
		if index < 0 {
			break
		}
		starts = append(starts, from+index)
		from += index + len(marker)
	}
	cards := make([]string, 0, len(starts))
	for i, start := range starts {
		end := len(source)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		cards = append(cards, source[start:end])
	}
	return cards
}

// cardMatches reports whether a /movie/<slug>-<id> path names the wanted code.
func cardMatches(moviePath, want string) bool {
	slug := moviePath
	if match := slugCodeRe.FindStringSubmatch(moviePath); match != nil {
		slug = match[1]
	}
	return codeid.IsFormatEquivalent(slug, want)
}

// decodeDn pulls the magnet display name (dn) out of a magnet href and decodes
// its percent-encoding and HTML entities.
func decodeDn(href string) string {
	index := strings.Index(href, "dn=")
	if index < 0 {
		return ""
	}
	value := href[index+len("dn="):]
	if cut := strings.IndexByte(value, '&'); cut >= 0 {
		value = value[:cut]
	}
	value = html.UnescapeString(value)
	if decoded, err := url.QueryUnescape(value); err == nil {
		return decoded
	}
	return value
}

// parseSize converts a magnet row's size cell ("5.2gb") to bytes.
func parseSize(cell string) int64 {
	match := sizeRe.FindStringSubmatch(cell)
	if match == nil {
		return 0
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0
	}
	factor := float64(1)
	switch strings.ToLower(match[2]) {
	case "tb":
		factor = 1 << 40
	case "gb":
		factor = 1 << 30
	case "mb":
		factor = 1 << 20
	case "kb":
		factor = 1 << 10
	}
	return int64(value * factor)
}

// parseDate converts a magnet row's dd/mm/yy date to ISO yyyy-mm-dd.
func parseDate(cell string) string {
	match := dateRe.FindStringSubmatch(cell)
	if match == nil {
		return ""
	}
	return "20" + match[3] + "-" + match[2] + "-" + match[1]
}
