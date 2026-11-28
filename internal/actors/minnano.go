package actors

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	cloudscraper "github.com/Advik-B/cloudscraper/lib"
	"github.com/Advik-B/cloudscraper/lib/proxy"
	useragent "github.com/Advik-B/cloudscraper/lib/user_agent"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

// MinNano adapts minnano-av.com actress profiles (aliases, birthday, sizes,
// blood type, birthplace, hobbies, debut work and a portrait). The site answers
// actress searches with a Cloudflare challenge, so requests go through
// cloudscraper, which keeps one solved session per proxy.
type MinNano struct {
	proxy *netx.ProxyManager
	base  string

	mu       sync.Mutex
	scraper  *cloudscraper.Scraper
	proxyKey string
	// getMu serializes requests: the library swaps Transport.Proxy per request
	// without synchronization, so concurrent Gets on one scraper race.
	getMu sync.Mutex
}

func NewMinNano(proxy *netx.ProxyManager) *MinNano {
	return &MinNano{proxy: proxy, base: "https://www.minnano-av.com/"}
}

func (*MinNano) Name() string { return "minnano-av" }

func (m *MinNano) Lookup(ctx context.Context, name string) (*Profile, error) {
	scraper, err := m.scraperFor()
	if err != nil {
		return nil, err
	}
	searchURL := m.base + "search_result.php?search_scope=actress&search_word=" + url.QueryEscape(name) + "&search=search"
	body, err := m.get(ctx, scraper, searchURL)
	if err != nil {
		return nil, err
	}
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	// An exact hit renders the profile page itself; canonical names the actress.
	if id := minnanoPageID(doc); id != "" {
		return minnanoProfile(doc, m.base+"actress"+id+".html", id)
	}
	// A loose hit renders the result list; open the matching actress page.
	if minnanoIsList(doc) {
		id := minnanoListMatch(doc, name)
		if id == "" {
			// The list is a valid response without our exact name, so this is a
			// miss, not a challenge.
			return nil, ErrProviderNotFound
		}
		pageURL := m.base + "actress" + id + ".html"
		body, err := m.get(ctx, scraper, pageURL)
		if err != nil {
			return nil, err
		}
		doc, err := html.Parse(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		if minnanoPageID(doc) != id {
			return nil, fmt.Errorf("minnano-av actress page %s is unrecognizable", id)
		}
		return minnanoProfile(doc, pageURL, id)
	}
	// Anything else (challenge or layout change) is an error so enrichment
	// retries later.
	return nil, fmt.Errorf("minnano-av search for %q returned an unrecognized page", name)
}

// scraperFor returns a scraper bound to the proxy currently configured, so a
// proxy change (and its new egress address) gets a fresh solved session.
func (m *MinNano) scraperFor() (*cloudscraper.Scraper, error) {
	proxyURL := ""
	if m.proxy != nil {
		if u := m.proxy.Resolve(); u != nil {
			proxyURL = u.String()
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.scraper != nil && m.proxyKey == proxyURL {
		return m.scraper, nil
	}
	options := []cloudscraper.ScraperOption{
		cloudscraper.WithBrowser(useragent.Config{Browser: "chrome", Platform: "windows", Desktop: true}),
		// Plain 403s recurse without bound inside the library's 403 retry, so
		// let them surface as errors and retry on the next enrichment instead.
		// Cloudflare challenges are detected before this path and still solved.
		cloudscraper.WithSessionConfig(false, time.Hour, 0),
	}
	if proxyURL != "" {
		options = append(options, cloudscraper.WithProxies([]string{proxyURL}, proxy.Sequential, 5*time.Minute))
	}
	scraper, err := cloudscraper.New(options...)
	if err != nil {
		return nil, err
	}
	m.scraper, m.proxyKey = scraper, proxyURL
	return scraper, nil
}

func (m *MinNano) get(ctx context.Context, scraper *cloudscraper.Scraper, target string) ([]byte, error) {
	m.getMu.Lock()
	defer m.getMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resp, err := scraper.Get(target)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("minnano-av status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

var minnanoActressURL = regexp.MustCompile(`actress(\d+)\.html`)

// minnanoPageID reports the actress id of a profile page, empty on list pages.
func minnanoPageID(doc *html.Node) string {
	for _, link := range nodes(doc, func(n *html.Node) bool {
		return n.Data == "link" && attr(n, "rel") == "canonical"
	}) {
		if m := minnanoActressURL.FindStringSubmatch(attr(link, "href")); m != nil {
			return m[1]
		}
	}
	for _, meta := range nodes(doc, func(n *html.Node) bool {
		return n.Data == "meta" && attr(n, "property") == "og:url"
	}) {
		if m := minnanoActressURL.FindStringSubmatch(attr(meta, "content")); m != nil {
			return m[1]
		}
	}
	return ""
}

// minnanoIsList reports whether the page is an actress result list.
func minnanoIsList(doc *html.Node) bool {
	if first(doc, func(n *html.Node) bool { return n.Data == "table" && hasClass(n, "tbllist") }) != nil {
		return true
	}
	return first(doc, func(n *html.Node) bool {
		return n.Data == "a" && minnanoActressURL.MatchString(attr(n, "href"))
	}) != nil
}

// minnanoListMatch finds the result-list entry whose name equals the search
// name, tolerating the spacing variants normalizeName drops.
func minnanoListMatch(doc *html.Node, name string) string {
	want := normalizeName(name)
	for _, link := range nodes(doc, func(n *html.Node) bool {
		return n.Data == "a" && minnanoActressURL.MatchString(attr(n, "href"))
	}) {
		label := text(link)
		if label == "" {
			if img := first(link, func(n *html.Node) bool { return n.Data == "img" }); img != nil {
				label = attr(img, "alt")
			}
		}
		if normalizeName(label) != want {
			continue
		}
		if m := minnanoActressURL.FindStringSubmatch(attr(link, "href")); m != nil {
			return m[1]
		}
	}
	return ""
}

// minnanoProfile reads the act-profile table of one actress page.
func minnanoProfile(doc *html.Node, pageURL, id string) (*Profile, error) {
	prof := first(doc, func(n *html.Node) bool { return n.Data == "div" && hasClass(n, "act-profile") })
	if prof == nil {
		return nil, fmt.Errorf("minnano-av actress page %s has no profile table", id)
	}
	p := &Profile{SourceID: id, SourceURL: pageURL}
	if h2 := first(prof, func(n *html.Node) bool { return n.Data == "h2" }); h2 != nil {
		p.Name = minnanoName(text(h2))
	}
	if p.Name == "" {
		return nil, fmt.Errorf("minnano-av actress page %s has no name", id)
	}
	for _, tr := range nodes(prof, func(n *html.Node) bool { return n.Data == "tr" }) {
		span := first(tr, func(n *html.Node) bool { return n.Data == "span" })
		if span == nil {
			continue
		}
		label := text(span)
		value := strings.TrimSpace(strings.TrimPrefix(text(tr), label))
		switch label {
		case "別名":
			if alias := minnanoAlias(value); alias != "" && alias != p.Name {
				p.Aliases = append(p.Aliases, alias)
			}
		case "生年月日":
			p.Birthday = minnanoDate(value)
		case "サイズ":
			p.Height, p.CupSize, p.Measurements = minnanoSizes(value)
		case "血液型":
			p.BloodType = strings.TrimSpace(strings.TrimSuffix(value, "型"))
		case "出身地":
			p.Nationality = value
		case "趣味・特技":
			p.Hobby = value
		case "デビュー作品":
			p.DebutDate = minnanoDate(value)
		}
	}
	p.Images = minnanoImages(doc, p.Name, pageURL)
	return p, nil
}

var (
	minnanoDateRe = regexp.MustCompile(`(\d{4})年\s*(\d{1,2})月\s*(\d{1,2})日`)
	minnanoCupRe  = regexp.MustCompile(`([A-Z])カップ`)
)

func minnanoDate(s string) string {
	m := minnanoDateRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", integer(m[1]), integer(m[2]), integer(m[3]))
}

// minnanoSizes parses "T159 / B88(Gカップ) / W57 / H83 / S23.5".
func minnanoSizes(s string) (height int, cup, measurements string) {
	var b, w, h int
	for _, item := range strings.Split(s, "/") {
		item = strings.TrimSpace(item)
		switch {
		case strings.HasPrefix(item, "T"):
			height = integer(item)
		case strings.HasPrefix(item, "B"):
			b = integer(item)
			if m := minnanoCupRe.FindStringSubmatch(item); m != nil {
				cup = m[1]
			}
		case strings.HasPrefix(item, "W"):
			w = integer(item)
		case strings.HasPrefix(item, "H"):
			h = integer(item)
		}
	}
	if b > 0 && w > 0 && h > 0 {
		measurements = fmt.Sprintf("B:%d / W:%d / H:%d", b, w, h)
	}
	return height, cup, measurements
}

// minnanoName keeps the main spelling of an h2 like
// "三上悠亜 （みかみゆあ / Mikami Yua）".
func minnanoName(s string) string {
	if i := strings.IndexAny(s, "（( "); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// minnanoAlias drops the kana/romanized parenthetical of an alias cell.
func minnanoAlias(s string) string {
	if i := strings.IndexAny(s, "（("); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// minnanoImages returns the actress portrait, preferring the image labelled
// with her name over the first portrait on the page.
func minnanoImages(doc *html.Node, name, base string) []string {
	fallback := ""
	for _, img := range nodes(doc, func(n *html.Node) bool { return n.Data == "img" }) {
		src := attr(img, "src")
		if !strings.Contains(src, "p_actress_125_125/") {
			continue
		}
		u := absoluteURL(base, src)
		if u == "" {
			continue
		}
		if attr(img, "title") == name || attr(img, "alt") == name {
			return []string{u}
		}
		if fallback == "" {
			fallback = u
		}
	}
	if fallback != "" {
		return []string{fallback}
	}
	return nil
}
