// Package freejavbt scrapes freejavbt.com: a code-addressable page that carries
// both catalogue metadata (title, date, actors, tags, artwork) and a
// server-rendered magnet table. It is used as a low-priority supplementary
// source for fields and magnets.
package freejavbt

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
)

// magnetEntry is one parsed magnet row from the detail page.
type magnetEntry struct {
	Hash        string
	Name        string
	HasSubtitle bool
	HD          bool
	FilesCount  int
	CreatedAt   string
}

// detail is the parsed movie page.
type detail struct {
	Code          string
	Title         string
	ReleaseDate   string
	Duration      int
	Actors        []string
	Tags          []string
	Cover         string
	Previews      []domain.PreviewImage
	Magnets       []magnetEntry
	ActorMovies   []domain.MovieReference
	RelatedMovies []domain.MovieReference
}

var (
	metaDescRe = regexp.MustCompile(`(?is)<meta\s+name="description"\s+content="([^"]*)"`)
	ogImageRe  = regexp.MustCompile(`(?is)<meta\s+property="og:image"\s+content="([^"]*)"`)
	coverRe    = regexp.MustCompile(`(?i)https://c0\.jdbstatic\.com/covers/[^"' ]+`)
	sampleRe   = regexp.MustCompile(`(?i)data-src="(https://c0\.jdbstatic\.com/samples/[^"]+)"`)

	rowRe     = regexp.MustCompile(`(?is)<tr>(.*?)</tr>`)
	hrefRe    = regexp.MustCompile(`(?i)href="(magnet:\?xt=urn:btih:[^"]+)"`)
	hashRe    = regexp.MustCompile(`(?i)urn:btih:([0-9a-f]{40})`)
	nameRe    = regexp.MustCompile(`(?is)<a[^>]*class="[^"]*magnet-name[^"]*"[^>]*>([^<]*)</a>`)
	tagRe     = regexp.MustCompile(`(?is)<span class="tag[^"]*">([^<]*)</span>`)
	rowDateRe = regexp.MustCompile(`(?is)<td class="date[^"]*">\s*<span>([0-9]{4}-[0-9]{2}-[0-9]{2})</span>`)
	filesRe   = regexp.MustCompile(`(\d+)\s*個文件`)

	codeRe  = regexp.MustCompile(`影片番號為([^，]+)`)
	titleRe = regexp.MustCompile(`影片名是([^，]+)`)
	dateRe  = regexp.MustCompile(`發佈日期為(\d{4}-\d{2}-\d{2})`)
	durRe   = regexp.MustCompile(`影片時長(\d+)分鐘`)
	actorRe = regexp.MustCompile(`主演女優是([^，]+)`)
	tagsRe  = regexp.MustCompile(`主題為([^。]+)`)

	cardRe = regexp.MustCompile(`(?is)<a href="https://freejavbt\.com/([A-Z0-9-]+)">(.*?)</a>`)
	imgRe  = regexp.MustCompile(`(?i)data-src="([^"]+)"`)
)

// parseDetail extracts the metadata and magnet rows from a freejavbt movie page.
func parseDetail(html string) detail {
	var result detail

	description := ""
	for _, match := range metaDescRe.FindAllStringSubmatch(html, -1) {
		if strings.Contains(match[1], "影片番號為") {
			description = match[1]
			break
		}
	}
	if description == "" {
		return result
	}

	result.Code = trimField(codeRe, description)
	result.Title = trimField(titleRe, description)
	result.ReleaseDate = trimField(dateRe, description)
	if minutes := trimField(durRe, description); minutes != "" {
		result.Duration, _ = strconv.Atoi(minutes)
	}
	result.Actors = splitList(trimField(actorRe, description))
	result.Tags = splitList(trimField(tagsRe, description))

	if match := ogImageRe.FindStringSubmatch(html); match != nil {
		result.Cover = strings.TrimSpace(match[1])
	}
	if result.Cover == "" {
		result.Cover = coverRe.FindString(html)
	}

	seen := make(map[string]bool)
	for _, match := range sampleRe.FindAllStringSubmatch(html, -1) {
		url := strings.TrimSpace(match[1])
		if url == "" || seen[url] {
			continue
		}
		seen[url] = true
		result.Previews = append(result.Previews, domain.PreviewImage{Thumbnail: url, Original: url})
	}

	for _, row := range rowRe.FindAllStringSubmatch(html, -1) {
		body := row[1]
		if !strings.Contains(body, "magnet-name") {
			continue
		}
		href := hrefRe.FindStringSubmatch(body)
		if href == nil {
			continue
		}
		hash := hashRe.FindStringSubmatch(href[1])
		if hash == nil {
			continue
		}
		entry := magnetEntry{Hash: strings.ToLower(hash[1])}
		if name := nameRe.FindStringSubmatch(body); name != nil {
			entry.Name = strings.TrimSpace(name[1])
		}
		for _, tag := range tagRe.FindAllStringSubmatch(body, -1) {
			label := strings.TrimSpace(tag[1])
			switch {
			case strings.Contains(label, "字幕"):
				entry.HasSubtitle = true
			case strings.Contains(label, "高清") || strings.Contains(label, "4K"):
				entry.HD = true
			}
		}
		if files := filesRe.FindStringSubmatch(body); files != nil {
			entry.FilesCount, _ = strconv.Atoi(files[1])
		}
		if date := rowDateRe.FindStringSubmatch(body); date != nil {
			entry.CreatedAt = date[1]
		}
		result.Magnets = append(result.Magnets, entry)
	}
	result.ActorMovies = parseRefs(section(html, "同女優作品"))
	result.RelatedMovies = parseRefs(section(html, "同主題作品"))
	return result
}

// section returns the HTML between a section heading (h2) and the next h2.
func section(html, heading string) string {
	marker := heading + "</h2>"
	index := strings.Index(html, marker)
	if index < 0 {
		return ""
	}
	rest := html[index+len(marker):]
	if cut := strings.Index(strings.ToLower(rest), "<h2"); cut >= 0 {
		rest = rest[:cut]
	}
	return rest
}

// parseRefs extracts movie references from a related-works grid.
func parseRefs(section string) []domain.MovieReference {
	const limit = 12
	refs := make([]domain.MovieReference, 0, limit)
	seen := make(map[string]bool)
	for _, match := range cardRe.FindAllStringSubmatch(section, -1) {
		code := match[1]
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		thumbnail := ""
		if image := imgRe.FindStringSubmatch(match[2]); image != nil {
			thumbnail = image[1]
		}
		refs = append(refs, domain.MovieReference{ID: code, Code: code, Thumbnail: thumbnail, Cover: thumbnail})
		if len(refs) >= limit {
			break
		}
	}
	return refs
}

func trimField(pattern *regexp.Regexp, text string) string {
	if match := pattern.FindStringSubmatch(text); match != nil {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func splitList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, "、")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if name := strings.TrimSpace(part); name != "" {
			result = append(result, name)
		}
	}
	return result
}
