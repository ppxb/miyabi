package providers

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

// Jav321 adapts jav321.com, a JavBus-class catalogue aggregator, into the
// metadata provider contract. Because our interface only carries a catalogue
// number, Fetch resolves the number to a detail URL through the site's POST
// search (which answers with a 301 to /video/<id>) and then parses the page.
type Jav321 struct{ client }

func NewJav321(proxy *netx.ProxyManager) *Jav321 {
	return &Jav321{newClient(proxy, "https://www.jav321.com/", "", "jav321.com")}
}

func (*Jav321) ID() string { return "jav321" }

const jav321UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

var jav321Star = regexp.MustCompile(`/star/([0-9]+)`)

// Supports mirrors the JavBus aggregator: any catalogue number except the FC2
// and HEYZO families, which dedicated sources handle.
func (*Jav321) Supports(code string) bool {
	upper := strings.ToUpper(code)
	return upper != "" && !strings.HasPrefix(upper, "FC2") && !strings.HasPrefix(upper, "HEYZO")
}

func (s *Jav321) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	if code == "" || !s.Supports(code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	detailURL, err := s.resolveDetailURL(ctx, code)
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	body, err := s.get(ctx, detailURL)
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return parseJav321(string(body), s.base, code)
}

// resolveDetailURL posts the catalogue number to the site search and returns the
// /video/<id> URL it redirects to. No redirect with a /video target means the
// site has no such entry.
func (s *Jav321) resolveDetailURL(ctx context.Context, keyword string) (string, error) {
	if err := s.admit(ctx); err != nil {
		return "", err
	}
	form := url.Values{"sn": {keyword}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.base+"search", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", jav321UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", s.base)

	// Read the redirect without following it; the shared client follows by default.
	redirectless := *s.http
	redirectless.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := redirectless.Do(req)
	if err != nil {
		return "", s.failed(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	switch resp.StatusCode {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
	default:
		// Rate limiting and server errors must feed the shared cooldown (honouring
		// Retry-After) instead of being cached as a plain miss.
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
			return "", s.failed(&domain.HTTPError{Source: "metadata", StatusCode: resp.StatusCode, RetryAfter: domain.ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())})
		}
		return "", metadata.ErrNotFound
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", metadata.ErrNotFound
	}
	target := absolute(s.base, loc)
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" || !jav321Host(u.Hostname()) || !strings.HasPrefix(u.Path, "/video/") {
		return "", metadata.ErrNotFound
	}
	return target, nil
}

func parseJav321(body, base, code string) (domain.MovieMetadata, error) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	number := trimColon(jav321Field(doc, "品番"))
	if number == "" || !codeid.IsEquivalent(number, code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	title := text(tag(doc, "h3"))
	if title == "" {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}

	m := domain.Movie{
		Code:        code,
		Title:       title,
		OriginTitle: title,
		ReleaseDate: releaseDate(trimColon(jav321Field(doc, "配信開始日"))),
		Duration:    integer(trimColon(jav321Field(doc, "収録時間"))),
		Actors:      jav321Actors(doc, base),
		Sources:     []domain.SourceID{{Provider: "jav321", ID: code}},
	}
	if value, err := strconv.ParseFloat(trimColon(jav321Field(doc, "平均評価")), 64); err == nil && value > 0 && value <= 5 {
		m.Rating, m.RatingMax, m.RatingSource = value, 5, "jav321"
	}
	if name, id := jav321Maker(doc); name != "" {
		m.Maker = &domain.Maker{Provider: "jav321", ID: id, Name: name}
	}

	images := jav321Images(doc, base)
	if len(images) > 0 {
		m.Cover, m.Thumbnail = images[0], images[0]
	}
	result := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: m}}
	for i, url := range images {
		role := "preview"
		if i == 0 {
			role = "cover"
		}
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "jav321", URL: url, Role: role})
		if i > 0 {
			m.PreviewImages = append(m.PreviewImages, domain.PreviewImage{Original: url, Thumbnail: url})
		}
	}
	result.Detail.PreviewImages = m.PreviewImages
	result.Detail.HasPreview = len(m.PreviewImages) > 0
	return result, nil
}

// jav321Field returns the text that follows a <b> label such as 品番.
func jav321Field(doc *html.Node, label string) string {
	for _, b := range nodes(doc, func(n *html.Node) bool { return n.Data == "b" && strings.Contains(text(n), label) }) {
		for s := b.NextSibling; s != nil; s = s.NextSibling {
			if s.Type == html.TextNode {
				if t := strings.TrimSpace(s.Data); t != "" {
					return t
				}
			}
			if s.Type == html.ElementNode {
				if t := text(s); t != "" {
					return t
				}
			}
		}
	}
	return ""
}

func jav321Maker(doc *html.Node) (name, id string) {
	for _, b := range nodes(doc, func(n *html.Node) bool { return n.Data == "b" && strings.Contains(text(n), "メーカー") }) {
		for s := b.NextSibling; s != nil; s = s.NextSibling {
			if s.Type == html.ElementNode && s.Data == "a" && strings.Contains(attr(s, "href"), "/company") {
				name = text(s)
				if name == "" {
					return "", ""
				}
				return name, name
			}
		}
	}
	return "", ""
}

func jav321Actors(doc *html.Node, base string) []domain.Actor {
	var actors []domain.Actor
	seen := make(map[string]bool)
	for _, a := range nodes(doc, func(n *html.Node) bool {
		return n.Data == "a" && strings.Contains(attr(n, "href"), "/star/")
	}) {
		name := text(a)
		parts := jav321Star.FindStringSubmatch(pathOf(absolute(base, attr(a, "href"))))
		if parts == nil || name == "" || seen[parts[1]] {
			continue
		}
		seen[parts[1]] = true
		actors = append(actors, domain.Actor{Provider: "jav321", ID: parts[1], Name: name})
	}
	return actors
}

// jav321Images reads the work's cover and sample screenshots from
// div.col-xs-12.col-md-12 > p > a > img. The img src points at pics.dmm.co.jp
// over plain http; the site mirrors the same path under its own https host, so
// rewrite to that mirror for a stable, referer-free fetch.
func jav321Images(doc *html.Node, base string) []string {
	var images []string
	seen := make(map[string]bool)
	for _, div := range nodes(doc, func(n *html.Node) bool {
		return n.Data == "div" && hasClass(n, "col-xs-12") && hasClass(n, "col-md-12")
	}) {
		for _, p := range nodes(div, func(n *html.Node) bool { return n.Data == "p" }) {
			for _, a := range nodes(p, func(n *html.Node) bool { return n.Data == "a" }) {
				for _, img := range nodes(a, func(n *html.Node) bool { return n.Data == "img" }) {
					mirror := jav321Mirror(base, attr(img, "src"))
					if mirror == "" || seen[mirror] {
						continue
					}
					seen[mirror] = true
					images = append(images, mirror)
				}
			}
		}
	}
	return images
}

func jav321Mirror(base, src string) string {
	if src == "" {
		return ""
	}
	u, err := url.Parse(absolute(base, src))
	if err != nil || (u.Hostname() != "" && u.Hostname() != "dmm.co.jp" && !strings.HasSuffix(u.Hostname(), ".dmm.co.jp") && !jav321Host(u.Hostname())) {
		return ""
	}
	path := u.Path
	for strings.HasPrefix(path, "//") {
		path = path[1:]
	}
	if path == "" {
		return ""
	}
	return "https://www.jav321.com" + path
}

func jav321Host(host string) bool {
	return host == "jav321.com" || strings.HasSuffix(host, ".jav321.com")
}

func trimColon(value string) string {
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(value), ":"))
}
