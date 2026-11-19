package providers

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

// Caribbeancom adapts www.caribbeancom.com (EUC-JP) into the metadata provider
// contract. Its Premium sibling (caribbeancompr) shares the same page layout.
type Caribbeancom struct{ client }

func NewCaribbeancom(proxy *netx.ProxyManager) *Caribbeancom {
	return &Caribbeancom{newClient(proxy, "https://www.caribbeancom.com/", "", "caribbeancom.com", "caribbeancompr.com")}
}

func (*Caribbeancom) ID() string { return "caribbeancom" }

var (
	caribbeanDateCode   = regexp.MustCompile(`^\d{6}[-_]\d{3}$`)
	caribbeanDateFromID = regexp.MustCompile(`^(\d{6})[-_]\d+`)
	caribbeanEmimg      = regexp.MustCompile(`(?i)emimg\s*=\s*'([^']+)'`)
	caribbeanPoster     = regexp.MustCompile(`posterImage\s*=\s*'([^']+)'\s*\+\s*movie_id\s*\+\s*'([^']+)'`)
)

func (*Caribbeancom) Supports(code string) bool { return caribbeanDateCode.MatchString(code) }

func (s *Caribbeancom) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	if !caribbeanDateCode.MatchString(code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	siteID := strings.ReplaceAll(code, "_", "-")
	body, err := s.get(ctx, s.base+"moviepages/"+url.PathEscape(siteID)+"/index.html")
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return parseCaribbean(decodeHTML(body), s.base, code, siteID, "caribbeancom", "カリビアンコム")
}

// parseCaribbean parses a moviepages page shared by caribbeancom and
// caribbeancompr. siteID is the id used in image paths and URLs.
func parseCaribbean(decoded, base, code, siteID, providerID, defaultMaker string) (domain.MovieMetadata, error) {
	doc, err := html.Parse(strings.NewReader(decoded))
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	pages := elementByID(doc, "moviepages")
	// Hard identity: the detail container must exist and the page must reference
	// the requested id somewhere (image/script URLs). A soft 404 lacks both.
	if pages == nil || !strings.Contains(decoded, siteID) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}

	title := ""
	if h1 := first(doc, func(n *html.Node) bool { return n.Data == "h1" && attr(n, "itemprop") == "name" }); h1 != nil {
		title = strings.TrimSpace(text(h1))
	}
	if title == "" {
		if h1 := tag(pages, "h1"); h1 != nil {
			title = strings.TrimSpace(text(h1))
		}
	}
	if title == "" {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	plot := ""
	if p := first(doc, func(n *html.Node) bool { return n.Data == "p" && attr(n, "itemprop") == "description" }); p != nil {
		plot = strings.TrimSpace(text(p))
	} else if p := tag(pages, "p"); p != nil {
		plot = strings.TrimSpace(text(p))
	}

	m := domain.Movie{
		Code: code, Title: title, OriginTitle: title, Plot: plot,
		Maker:   &domain.Maker{Provider: providerID, ID: defaultMaker, Name: defaultMaker},
		Sources: []domain.SourceID{{Provider: providerID, ID: code}},
	}
	seenActor, seenTag := make(map[string]bool), make(map[string]bool)
	for _, li := range nodes(pages, func(n *html.Node) bool { return n.Data == "li" }) {
		spans := nodes(li, func(n *html.Node) bool { return n.Data == "span" })
		if len(spans) < 2 {
			continue
		}
		label, box := strings.TrimRight(text(spans[0]), "：: "), spans[1]
		switch label {
		case "出演":
			for _, name := range linkOrNames(box) {
				if !seenActor[name] {
					seenActor[name] = true
					m.Actors = append(m.Actors, domain.Actor{Provider: providerID, ID: name, Name: name})
				}
			}
		case "タグ":
			for _, name := range linkOrNames(box) {
				if !seenTag[name] {
					seenTag[name] = true
					m.Tags = append(m.Tags, domain.Tag{Provider: providerID, ID: name, Name: name})
				}
			}
		case "配信日", "販売日":
			m.ReleaseDate = releaseDate(text(box))
		case "再生時間":
			m.Duration = japaneseDuration(text(box))
		case "シリーズ":
			if name := linkOrNames(box); len(name) > 0 {
				m.Series = &domain.Series{Provider: providerID, ID: name[0], Name: name[0]}
			}
		case "スタジオ":
			if name := linkOrNames(box); len(name) > 0 {
				m.Maker = &domain.Maker{Provider: providerID, ID: name[0], Name: name[0]}
			}
		}
	}
	if m.ReleaseDate == "" {
		m.ReleaseDate = caribbeanDateFallback(siteID)
	}

	cover := caribbeanCover(decoded, base, siteID)
	m.Cover, m.Thumbnail = cover, cover

	result := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: m, Zone: domain.ZoneUncensored}}
	if m.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: providerID, URL: m.Cover, Role: "cover", Layout: domain.CoverSingle})
	}
	seen := make(map[string]bool)
	for _, div := range nodes(doc, func(n *html.Node) bool { return hasClass(n, "gallery-ratio") }) {
		for _, a := range nodes(div, func(n *html.Node) bool { return n.Data == "a" }) {
			href := attr(a, "href")
			if href == "" || strings.Contains(href, "member") {
				continue
			}
			u := caribbeanImage(base, href)
			if u == "" || seen[u] {
				continue
			}
			seen[u] = true
			result.Images = append(result.Images, domain.ImageCandidate{Provider: providerID, URL: u, Role: "preview"})
			m.PreviewImages = append(m.PreviewImages, domain.PreviewImage{Original: u, Thumbnail: u})
		}
	}
	result.Detail.PreviewImages = m.PreviewImages
	result.Detail.HasPreview = len(m.PreviewImages) > 0
	return result, nil
}

// linkOrNames returns linked names when present, otherwise whitespace/punctuation
// split text with leading/trailing hyphens trimmed (caribbeancom separates actors
// with " - ").
func linkOrNames(box *html.Node) []string {
	if links := nodes(box, func(n *html.Node) bool { return n.Data == "a" }); len(links) > 0 {
		var names []string
		for _, a := range links {
			if name := strings.Trim(strings.TrimSpace(text(a)), "-"); name != "" {
				names = append(names, name)
			}
		}
		return names
	}
	var names []string
	for _, name := range splitNames(text(box)) {
		if name = strings.Trim(name, "-"); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func caribbeanDateFallback(siteID string) string {
	digits := caribbeanDateFromID.FindStringSubmatch(siteID)
	if digits == nil {
		return ""
	}
	t, err := time.Parse("010206", digits[1])
	if err != nil {
		return ""
	}
	return t.Format(time.DateOnly)
}

// caribbeanCover reads the cover from the page scripts: either emimg or a
// posterImage template that concatenates the movie id.
func caribbeanCover(decoded, base, siteID string) string {
	if m := caribbeanEmimg.FindStringSubmatch(decoded); m != nil {
		if u := caribbeanImage(base, m[1]); u != "" {
			return u
		}
	}
	if m := caribbeanPoster.FindStringSubmatch(decoded); m != nil {
		if u := caribbeanImage(base, m[1]+siteID+m[2]); u != "" {
			return u
		}
	}
	return ""
}

func caribbeanHost(host string) bool {
	return host == "caribbeancom.com" || strings.HasSuffix(host, ".caribbeancom.com") ||
		host == "caribbeancompr.com" || strings.HasSuffix(host, ".caribbeancompr.com")
}

func caribbeanImage(base, ref string) string {
	if ref == "" {
		return ""
	}
	u, err := url.Parse(absolute(base, ref))
	if err != nil || u.Scheme != "https" || u.User != nil || !caribbeanHost(u.Hostname()) {
		return ""
	}
	return u.String()
}
