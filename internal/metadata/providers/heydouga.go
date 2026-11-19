package providers

import (
	"context"
	"net/url"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

// Heydouga adapts heydouga.com movie pages into the metadata provider contract.
// Ids are the two-part "4030-1938" family. The page is UTF-8; the structured
// fields live in li>span pairs and require the site's over-18 consent cookies.
type Heydouga struct{ client }

func NewHeydouga(proxy *netx.ProxyManager) *Heydouga {
	return &Heydouga{newClient(proxy, "https://www.heydouga.com/", "lang=ja; over18_ppv=1; feature_group=1", "heydouga.com")}
}

func (*Heydouga) ID() string { return "heydouga" }

func (*Heydouga) Supports(code string) bool {
	return strings.HasPrefix(strings.ToUpper(code), "HEYDOUGA-")
}

func (s *Heydouga) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	if code == "" || !s.Supports(code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	rest := strings.TrimPrefix(strings.ToUpper(code), "HEYDOUGA-")
	parts := strings.SplitN(rest, "-", 2)
	if len(parts) != 2 || !isAllDigits(parts[0]) || !isAllDigits(parts[1]) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	body, err := s.get(ctx, s.base+"moviepages/"+url.PathEscape(parts[0])+"/"+url.PathEscape(parts[1])+"/index.html")
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return parseHeydouga(string(body), s.base, parts[0], parts[1])
}

func parseHeydouga(body, base, part1, part2 string) (domain.MovieMetadata, error) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	// The page must claim this very work: og:url (or canonical) has to point at
	// /moviepages/<p1>/<p2>/index.html. This blocks soft 404 / consent pages.
	wantPath := "/moviepages/" + part1 + "/" + part2 + "/index.html"
	confirmed := false
	if ref := firstNonEmpty(metaContent(doc, "og:url"), canonicalHref(doc)); ref != "" {
		if u, err := url.Parse(absolute(base, ref)); err == nil && heydougaHost(u.Hostname()) && u.Path == wantPath {
			confirmed = true
		} else {
			return domain.MovieMetadata{}, metadata.ErrNotFound
		}
	}

	title := ""
	if titleBg := elementByID(doc, "title-bg"); titleBg != nil {
		if h1 := tag(titleBg, "h1"); h1 != nil {
			title = strings.TrimSpace(text(h1))
		}
	}
	if !confirmed || title == "" {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}

	id := part1 + "-" + part2
	m := domain.Movie{
		Code: "HEYDOUGA-" + id, Title: title, OriginTitle: title,
		Sources: []domain.SourceID{{Provider: "heydouga", ID: id}},
	}
	if desc := elementByID(doc, "movie-description"); desc != nil {
		m.Plot = text(desc)
	}

	seenActor := make(map[string]bool)
	for _, li := range nodes(doc, func(n *html.Node) bool { return n.Data == "li" }) {
		spans := nodes(li, func(n *html.Node) bool { return n.Data == "span" })
		if len(spans) == 0 {
			continue
		}
		label := strings.TrimRight(text(spans[0]), "：: ")
		var value string
		if len(spans) >= 2 {
			value = strings.TrimSpace(text(spans[1]))
		} else {
			value = strings.TrimSpace(strings.TrimPrefix(text(li), text(spans[0])))
		}
		value = strings.TrimSpace(strings.TrimLeft(value, "：: "))
		switch label {
		case "配信日":
			m.ReleaseDate = releaseDate(value)
		case "配信期間":
			if i := strings.IndexAny(value, "～~"); i >= 0 {
				value = value[:i]
			}
			m.ReleaseDate = releaseDate(value)
		case "主演":
			for _, name := range splitNames(value) {
				if !seenActor[name] {
					seenActor[name] = true
					m.Actors = append(m.Actors, domain.Actor{Provider: "heydouga", ID: name, Name: name})
				}
			}
		case "提供元":
			if maker := cleanMaker(value); maker != "" {
				m.Maker = &domain.Maker{Provider: "heydouga", ID: maker, Name: maker}
			}
		case "動画再生時間":
			m.Duration = japaneseDuration(value)
		}
	}

	if cover := heydougaImage(base, metaContent(doc, "og:image")); cover != "" {
		m.Cover = cover
		m.Thumbnail = cover
	}

	result := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: m, Zone: domain.ZoneUncensored}}
	if m.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "heydouga", URL: m.Cover, Role: "cover", Layout: domain.CoverSingle})
	}
	return result, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func canonicalHref(doc *html.Node) string {
	return attr(first(doc, func(n *html.Node) bool { return n.Data == "link" && attr(n, "rel") == "canonical" }), "href")
}

func metaContent(doc *html.Node, property string) string {
	return attr(first(doc, func(n *html.Node) bool { return n.Data == "meta" && attr(n, "property") == property }), "content")
}

func cleanMaker(v string) string {
	if i := strings.IndexAny(v, "（("); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(v), "：: "))
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func heydougaHost(host string) bool {
	return host == "heydouga.com" || strings.HasSuffix(host, ".heydouga.com")
}

func heydougaImage(base, ref string) string {
	if ref == "" {
		return ""
	}
	u, err := url.Parse(absolute(base, ref))
	if err != nil || u.Scheme != "https" || u.User != nil || !heydougaHost(u.Hostname()) {
		return ""
	}
	return u.String()
}
