package actors

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

// AVLeague adapts av-league.com actress profiles (birthday, height, measurements,
// cup, blood type, nationality, debut date, aliases, images).
type AVLeague struct {
	http *http.Client
	base string
}

func NewAVLeague(proxy *netx.ProxyManager) *AVLeague {
	return &AVLeague{http: netx.NewSafeDownloadClient(proxy, 25*time.Second), base: "https://www.av-league.com/"}
}

func (*AVLeague) Name() string { return "av-league" }

func (a *AVLeague) Lookup(ctx context.Context, name string) (*Profile, error) {
	id, err := a.searchID(ctx, name)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, ErrProviderNotFound
	}
	return a.fetch(ctx, id)
}

func (a *AVLeague) get(ctx context.Context, target string) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept-Language", "ja")
		resp, err := a.http.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusOK {
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
			resp.Body.Close()
			return data, readErr
		}
		lastErr = fmt.Errorf("av-league status %d", resp.StatusCode)
		resp.Body.Close()
		// Back off on rate limiting / transient server errors.
		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			return nil, lastErr
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt) * 2 * time.Second):
		}
	}
	return nil, lastErr
}

func (a *AVLeague) searchID(ctx context.Context, name string) (string, error) {
	body, err := a.get(ctx, a.base+"search/search.php?k="+url.QueryEscape(name))
	if err != nil {
		return "", err
	}
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	want := normalizeName(name)
	for _, e := range nodes(doc, func(n *html.Node) bool {
		return n.Data == "div" && hasClass(n, "l-name")
	}) {
		link := first(e, func(n *html.Node) bool { return n.Data == "a" })
		if link == nil {
			continue
		}
		if normalizeName(text(link)) != want {
			continue
		}
		href := attr(link, "href")
		if i := strings.LastIndex(href, "/"); i >= 0 {
			id := strings.TrimSuffix(href[i+1:], ".html")
			if id != "" {
				return id, nil
			}
		}
	}
	return "", nil
}

func (a *AVLeague) fetch(ctx context.Context, id string) (*Profile, error) {
	pageURL := a.base + "actress/" + id + ".html"
	body, err := a.get(ctx, pageURL)
	if err != nil {
		return nil, err
	}
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	p := &Profile{SourceID: id, SourceURL: pageURL}
	if pan := elementByID(doc, "pan"); pan != nil {
		if span := tag(pan, "span"); span != nil {
			p.Name = strings.TrimSpace(text(span))
		} else {
			p.Name = strings.TrimSpace(text(pan))
		}
	}
	if p.Name == "" {
		return nil, ErrProviderNotFound
	}
	if prof := elementByID(doc, "j-prof"); prof != nil {
		for _, span := range nodes(prof, func(n *html.Node) bool { return n.Data == "span" }) {
			t := text(span)
			if name, aliases, found := strings.Cut(t, ":"); found && strings.Contains(name, "別名") {
				for _, alias := range strings.Split(aliases, "、") {
					if alias = strings.TrimSpace(alias); alias != "" {
						p.Aliases = append(p.Aliases, alias)
					}
				}
			}
		}
	}
	contents := elementByID(doc, "contents")
	for _, tr := range nodes(contents, func(n *html.Node) bool { return n.Data == "tr" }) {
		th := first(tr, func(n *html.Node) bool { return n.Data == "th" })
		td := first(tr, func(n *html.Node) bool { return n.Data == "td" })
		if th == nil || td == nil {
			continue
		}
		label, value := strings.TrimSpace(text(th)), strings.TrimSpace(text(td))
		switch label {
		case "3サイズ":
			p.Measurements, p.CupSize = parseMeasurements(value)
		case "身長":
			p.Height = integer(strings.TrimRight(value, "cm"))
		case "血液型":
			p.BloodType = strings.TrimRight(value, "型")
		case "生年月日":
			p.Birthday = japaneseDate(value)
		case "出身":
			p.Nationality = value
		case "デビュー":
			p.DebutDate = japaneseDate(value)
		}
	}
	p.Images = avLeagueImages(doc, a.base)
	return p, nil
}

func parseMeasurements(s string) (measurements, cup string) {
	var b, w, h int
	for _, item := range strings.Split(s, "/") {
		key, data, found := strings.Cut(item, ":")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "B":
			if ss := avLeagueMeasurements.FindStringSubmatch(data); len(ss) == 3 {
				b, _ = strconv.Atoi(ss[1])
				cup = ss[2]
			}
		case "W":
			w = integer(data)
		case "H":
			h = integer(data)
		}
	}
	if b != 0 && w != 0 && h != 0 {
		measurements = fmt.Sprintf("B:%d / W:%d / H:%d", b, w, h)
	}
	return measurements, cup
}

var avLeagueMeasurements = regexp.MustCompile(`(\d+)（([A-Z])）`)

var avLeagueDate = regexp.MustCompile(`^(\d{4})年(\d{1,2})月(\d{1,2})日`)

func japaneseDate(s string) string {
	if m := avLeagueDate.FindStringSubmatch(strings.TrimSpace(s)); m != nil {
		return fmt.Sprintf("%04d-%02d-%02d", integer(m[1]), integer(m[2]), integer(m[3]))
	}
	return ""
}

// avLeagueMaxImages caps the gallery so a prolific actress doesn't flood it.
const avLeagueMaxImages = 20

// avLeagueImages collects the gallery: the profile photo(s) in .i-pic-box, then
// the work captures (/image/cap_*) in page order, deduplicated and capped.
func avLeagueImages(doc *html.Node, base string) []string {
	seen := make(map[string]bool)
	var images []string
	add := func(ref string) {
		if len(images) >= avLeagueMaxImages || ref == "" || strings.HasPrefix(ref, "data:") {
			return
		}
		u := absoluteURL(base, ref)
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
		images = append(images, u)
	}
	for _, box := range nodes(doc, func(n *html.Node) bool { return hasClass(n, "i-pic-box") }) {
		for _, img := range nodes(box, func(n *html.Node) bool { return n.Data == "img" }) {
			for _, k := range []string{"src", "data-layzr", "data-src"} {
				add(attr(img, k))
			}
		}
	}
	for _, img := range nodes(doc, func(n *html.Node) bool {
		return n.Data == "img" && strings.Contains(attr(n, "src"), "/image/cap_")
	}) {
		add(attr(img, "src"))
	}
	return images
}

func absoluteURL(base, ref string) string {
	if ref == "" {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	return b.ResolveReference(u).String()
}
