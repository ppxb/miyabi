package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

// Duga adapts duga.jp/ppv (UTF-8), a large PPV storefront. It is a redundant
// source for standard label codes (e.g. glory-4262). Because the store spans
// many labels, Supports is broad; it can be disabled in settings if undesired.
type Duga struct{ client }

func NewDuga(proxy *netx.ProxyManager) *Duga {
	return &Duga{newClient(proxy, "https://duga.jp/", "", "duga.jp")}
}

func (*Duga) ID() string { return "duga" }

var dugaCode = regexp.MustCompile(`^[A-Z]{2,10}-\d{2,6}$`)

func (*Duga) Supports(code string) bool {
	upper := strings.ToUpper(code)
	return dugaCode.MatchString(upper) && !strings.HasPrefix(upper, "FC2") && !strings.HasPrefix(upper, "HEYZO")
}

func (s *Duga) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	if !s.Supports(code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	siteID := strings.ToLower(code)
	body, err := s.get(ctx, s.base+"ppv/"+url.PathEscape(siteID)+"/")
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return parseDuga(decodeHTML(body), s.base, code, siteID, "duga")
}

func parseDuga(decoded, base, code, siteID, providerID string) (domain.MovieMetadata, error) {
	doc, err := html.Parse(strings.NewReader(decoded))
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	nameBox := elementByID(doc, "contentsname")
	if nameBox == nil {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	// Page-side identity: og:url/canonical must point at this work when present.
	if ref := firstNonEmpty(metaContent(doc, "og:url"), canonicalHref(doc)); ref != "" {
		if u, err := url.Parse(absolute(base, ref)); err != nil || !dugaHost(u.Hostname()) || u.Path != "/ppv/"+siteID+"/" {
			return domain.MovieMetadata{}, metadata.ErrNotFound
		}
	}
	title := strings.TrimSpace(text(nameBox))
	if title == "" {
		title = metaContent(doc, "og:title")
	}
	if title == "" {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}

	m := domain.Movie{Code: code, Title: title, OriginTitle: title, Sources: []domain.SourceID{{Provider: providerID, ID: code}}}

	// Summary from JSON-LD description, fallback og:description.
	for _, script := range nodes(doc, func(n *html.Node) bool {
		return n.Data == "script" && attr(n, "type") == "application/ld+json"
	}) {
		var data struct {
			Description string `json:"description"`
		}
		if json.Unmarshal([]byte(strings.ReplaceAll(scriptText(script), "\n", "")), &data) == nil && data.Description != "" {
			m.Plot = data.Description
			break
		}
	}
	if m.Plot == "" {
		m.Plot = metaContent(doc, "og:description")
	}

	// Fields live across several tables; scan every th/td row. Release date
	// explicitly prefers 配信開始日 over 発売日.
	haisin, hatsubai := "", ""
	for _, tr := range nodes(doc, func(n *html.Node) bool { return n.Data == "tr" }) {
		th := first(tr, func(n *html.Node) bool { return n.Data == "th" })
		td := first(tr, func(n *html.Node) bool { return n.Data == "td" })
		if th == nil || td == nil {
			continue
		}
		value := strings.TrimSpace(text(td))
		switch strings.TrimSpace(text(th)) {
		case "配信開始日":
			if haisin == "" {
				haisin = dugaDate(value)
			}
		case "発売日":
			if hatsubai == "" {
				hatsubai = dugaDate(value)
			}
		case "メーカー":
			if m.Maker == nil && value != "" {
				m.Maker = &domain.Maker{Provider: providerID, ID: value, Name: value}
			}
		case "シリーズ":
			if m.Series == nil && value != "" {
				m.Series = &domain.Series{Provider: providerID, ID: value, Name: value}
			}
		case "再生時間":
			if m.Duration == 0 {
				m.Duration = japaneseDuration(value)
			}
		}
	}
	m.ReleaseDate = firstNonEmpty(haisin, hatsubai)

	if ul := first(doc, func(n *html.Node) bool { return n.Data == "ul" && hasClass(n, "performer") }); ul != nil {
		for _, a := range nodes(ul, func(n *html.Node) bool { return n.Data == "a" }) {
			if name := strings.TrimSpace(text(a)); name != "" {
				m.Actors = append(m.Actors, domain.Actor{Provider: providerID, ID: name, Name: name})
			}
		}
	}
	if ul := first(doc, func(n *html.Node) bool { return n.Data == "ul" && hasClass(n, "categorylist") }); ul != nil {
		for _, a := range nodes(ul, func(n *html.Node) bool { return n.Data == "a" }) {
			if name := strings.TrimSpace(text(a)); name != "" {
				m.Tags = append(m.Tags, domain.Tag{Provider: providerID, ID: name, Name: name})
			}
		}
	}
	if ul := first(doc, func(n *html.Node) bool { return n.Data == "ul" && hasClass(n, "director") }); ul != nil {
		if a := first(ul, func(n *html.Node) bool { return n.Data == "a" }); a != nil {
			if name := strings.TrimSpace(text(a)); name != "" {
				m.Director = &domain.Director{Provider: providerID, ID: name, Name: name}
			}
		}
	}

	if box := first(doc, func(n *html.Node) bool { return n.Data == "div" && hasClass(n, "imagebox") }); box != nil {
		if a := first(box, func(n *html.Node) bool { return n.Data == "a" }); a != nil {
			if u := dugaImage(base, attr(a, "href")); u != "" && isImagePath(u) {
				m.Cover = u
			}
		}
	}
	if m.Cover == "" {
		if img := first(doc, func(n *html.Node) bool { return n.Data == "img" && attr(n, "id") == "productjpg" }); img != nil {
			if u := dugaImage(base, attr(img, "src")); u != "" {
				m.Cover = u
			}
		}
	}
	m.Thumbnail = m.Cover

	result := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: m, Zone: domain.ZoneCensored}}
	if m.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: providerID, URL: m.Cover, Role: "cover", Layout: domain.CoverSingle})
	}
	if digest := elementByID(doc, "digestthumbbox"); digest != nil {
		seen := make(map[string]bool)
		for _, a := range nodes(digest, func(n *html.Node) bool { return n.Data == "a" }) {
			u := dugaImage(base, attr(a, "href"))
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

var dugaDateRe = regexp.MustCompile(`^(\d{4})年(\d{1,2})月(\d{1,2})日`)

// dugaDate accepts DUGA's Japanese date ("2022年06月06日") or a plain date.
func dugaDate(value string) string {
	if m := dugaDateRe.FindStringSubmatch(strings.TrimSpace(value)); m != nil {
		return fmt.Sprintf("%04d-%02d-%02d", integer(m[1]), integer(m[2]), integer(m[3]))
	}
	return releaseDate(value)
}

func dugaHost(host string) bool {
	return host == "duga.jp" || strings.HasSuffix(host, ".duga.jp")
}

func dugaImage(base, ref string) string {
	if ref == "" {
		return ""
	}
	u, err := url.Parse(absolute(base, ref))
	if err != nil || u.Scheme != "https" || u.User != nil || !dugaHost(u.Hostname()) {
		return ""
	}
	return u.String()
}
