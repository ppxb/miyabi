package providers

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

// Dahlia adapts dahlia-av.jp (UTF-8). Covers and previews are served from
// cdn.faleno.net.
type Dahlia struct{ client }

func NewDahlia(proxy *netx.ProxyManager) *Dahlia {
	return &Dahlia{newClient(proxy, "https://dahlia-av.jp/", "modal=off", "dahlia-av.jp", "faleno.net")}
}

func (*Dahlia) ID() string { return "dahlia" }

// Dahlia publishes the DLDSS series; kept narrow to avoid probing unrelated codes.
var dahliaCode = regexp.MustCompile(`(?i)^dldss-?\d+$`)

func (*Dahlia) Supports(code string) bool { return dahliaCode.MatchString(code) }

func (s *Dahlia) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	if !dahliaCode.MatchString(code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	siteID := strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(code))
	body, err := s.get(ctx, s.base+"works/"+url.PathEscape(siteID)+"/")
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return parseDahlia(decodeHTML(body), s.base, code, siteID, "dahlia", "DAHLIA")
}

func parseDahlia(decoded, base, code, siteID, providerID, defaultMaker string) (domain.MovieMetadata, error) {
	doc, err := html.Parse(strings.NewReader(decoded))
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	coverBox := first(doc, func(n *html.Node) bool { return n.Data == "div" && hasClass(n, "box_works01_img") })
	if coverBox == nil {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	// Page-side identity: when og:url/canonical is present it must point at this
	// work; a mismatched page is rejected outright.
	if ref := firstNonEmpty(metaContent(doc, "og:url"), canonicalHref(doc)); ref != "" {
		if u, err := url.Parse(absolute(base, ref)); err != nil || !dahliaHost(u.Hostname()) || u.Path != "/works/"+siteID+"/" {
			return domain.MovieMetadata{}, metadata.ErrNotFound
		}
	}
	title := ""
	for _, div := range nodes(doc, func(n *html.Node) bool {
		return n.Data == "div" && (hasClass(n, "bar02_works") || hasClass(n, "bar02"))
	}) {
		if h := tag(div, "h1"); h != nil {
			if t := strings.TrimSpace(text(h)); t != "" {
				title = t
				break
			}
		}
	}
	if title == "" {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}

	m := domain.Movie{
		Code: code, Title: title, OriginTitle: title,
		Maker:   &domain.Maker{Provider: providerID, ID: defaultMaker, Name: defaultMaker},
		Sources: []domain.SourceID{{Provider: providerID, ID: code}},
	}
	if textBox := first(doc, func(n *html.Node) bool { return n.Data == "div" && hasClass(n, "box_works01_text") }); textBox != nil {
		m.Plot = text(textBox)
	}

	seenActor := make(map[string]bool)
	haisin, hatsubai := "", ""
	for _, list := range nodes(doc, func(n *html.Node) bool { return n.Data == "div" && hasClass(n, "box_works01_list") }) {
		for _, sp := range nodes(list, func(n *html.Node) bool { return n.Data == "span" }) {
			label := strings.TrimSpace(text(sp))
			var p *html.Node
			if sp.Parent != nil {
				p = first(sp.Parent, func(n *html.Node) bool { return n.Data == "p" })
			}
			value := ""
			if p != nil {
				value = strings.TrimSpace(text(p))
			}
			switch label {
			case "出演女優":
				for _, name := range strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '／' }) {
					if name = strings.TrimSpace(name); name != "" && !seenActor[name] {
						seenActor[name] = true
						m.Actors = append(m.Actors, domain.Actor{Provider: providerID, ID: name, Name: name})
					}
				}
			case "収録時間":
				m.Duration = japaneseDuration(value)
			case "配信開始日":
				haisin = releaseDate(value)
			case "発売日":
				if hatsubai == "" {
					hatsubai = releaseDate(value)
				}
			}
		}
	}
	m.ReleaseDate = firstNonEmpty(haisin, hatsubai)

	// Cover from box_works01_img a img (strip query).
	if img := first(coverBox, func(n *html.Node) bool { return n.Data == "img" }); img != nil {
		if u := dahliaImage(base, attr(img, "src")); u != "" {
			m.Cover, m.Thumbnail = u, u
		}
	}

	result := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: m, Zone: domain.ZoneCensored}}
	if m.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: providerID, URL: m.Cover, Role: "cover", Layout: domain.CoverSingle})
	}
	seen := make(map[string]bool)
	for _, ga := range nodes(doc, func(n *html.Node) bool { return n.Data == "div" && hasClass(n, "box_works01_ga") }) {
		for _, a := range nodes(ga, func(n *html.Node) bool { return n.Data == "a" }) {
			u := dahliaImage(base, attr(a, "href"))
			if u == "" || !isImagePath(u) || seen[u] {
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

func dahliaHost(host string) bool {
	return host == "dahlia-av.jp" || strings.HasSuffix(host, ".dahlia-av.jp") ||
		host == "faleno.net" || strings.HasSuffix(host, ".faleno.net")
}

func dahliaImage(base, ref string) string {
	if ref == "" {
		return ""
	}
	u, err := url.Parse(absolute(base, ref))
	if err != nil || u.Scheme != "https" || u.User != nil || !dahliaHost(u.Hostname()) {
		return ""
	}
	u.RawQuery = ""
	return u.String()
}
