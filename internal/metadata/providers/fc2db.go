package providers

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

// FC2DB adapts fc2db.net work pages into the metadata provider contract.
// It is a redundant source for FC2-PPV numbers: the primary fc2 source reads the
// official product page, which carries the title and cover but no actresses,
// tags or seller. FC2DB supplies those missing fields.
type FC2DB struct{ client }

func NewFC2DB(proxy *netx.ProxyManager) *FC2DB {
	return &FC2DB{newClient(proxy, "https://fc2db.net/", "", "fc2db.net")}
}

func (*FC2DB) ID() string { return "fc2db" }

var (
	// h1 looks like: [FC2-PPV-4573362] Title...
	fc2dbTitle   = regexp.MustCompile(`^\[\s*FC2[-_\s]?PPV[-_\s]?(\d+)\s*\]\s*(.*)$`)
	fc2dbActress = regexp.MustCompile(`/actress/([0-9]+)/?$`)
	fc2dbTagPath = regexp.MustCompile(`/work-tags/([^/]+)/?$`)
)

// Supports reuses the FC2 number grammar: both the official fc2 source and this
// redundant source answer the same codes, so callers may keep either enabled.
func (*FC2DB) Supports(code string) bool { return fc2Code.MatchString(code) }

func (s *FC2DB) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	parts := fc2Code.FindStringSubmatch(ref.Code)
	if parts == nil {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	id := parts[1]
	body, err := s.get(ctx, s.base+"work/"+id+"/")
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return parseFC2DB(string(body), s.base, id)
}

func parseFC2DB(body, base, productID string) (domain.MovieMetadata, error) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	meta := func(property string) string {
		return attr(first(doc, func(n *html.Node) bool {
			return n.Data == "meta" && attr(n, "property") == property
		}), "content")
	}

	// An og:url that names a different work or host means we scraped the wrong
	// page. Fail loudly instead of trusting a coincidental h1.
	confirmed := false
	if ref := meta("og:url"); ref != "" {
		u, err := url.Parse(absolute(base, ref))
		if err != nil || !fc2dbHost(u.Hostname()) || strings.TrimSuffix(u.Path, "/") != "/work/"+productID {
			return domain.MovieMetadata{}, fmt.Errorf("FC2DB 作品页身份与请求不一致")
		}
		confirmed = true
	}

	title := ""
	for _, h1 := range nodes(doc, func(n *html.Node) bool { return n.Data == "h1" }) {
		raw := text(h1)
		if parts := fc2dbTitle.FindStringSubmatch(raw); parts != nil {
			if parts[1] != productID {
				return domain.MovieMetadata{}, fmt.Errorf("FC2DB 作品页编号与请求不一致")
			}
			title, confirmed = strings.TrimSpace(parts[2]), true
			break
		}
		if title == "" {
			title = stripBracketPrefix(raw)
		}
	}
	if title == "" {
		return domain.MovieMetadata{}, fmt.Errorf("FC2DB 作品页未返回标题")
	}

	m := domain.MovieDetail{Movie: domain.Movie{
		Code: "FC2-" + productID, Title: title, OriginTitle: title,
		Sources: []domain.SourceID{{Provider: "fc2db", ID: productID}},
	}, Zone: domain.ZoneFC2}

	var makerName string
	for _, dl := range nodes(doc, func(n *html.Node) bool { return n.Data == "dl" && hasClass(n, "grid") }) {
		for _, dt := range nodes(dl, func(n *html.Node) bool { return n.Data == "dt" }) {
			var dd *html.Node
			for sib := dt.NextSibling; sib != nil; sib = sib.NextSibling {
				if sib.Type == html.ElementNode && sib.Data == "dd" {
					dd = sib
					break
				}
			}
			if dd == nil {
				continue
			}
			label, value := text(dt), text(dd)
			switch {
			case strings.HasPrefix(label, "販売日"):
				m.ReleaseDate = releaseDate(value)
			case strings.HasPrefix(label, "商品ID"):
				if parts := fc2ProductID.FindStringSubmatch(strings.TrimSpace(value)); parts == nil || parts[1] != productID {
					return domain.MovieMetadata{}, fmt.Errorf("FC2DB 商品 ID 与请求不一致")
				}
				confirmed = true
			case strings.HasPrefix(label, "作品販売者"):
				makerName = strings.TrimSpace(value)
			}
		}
	}
	if !confirmed {
		return domain.MovieMetadata{}, fmt.Errorf("FC2DB 作品页缺少可确认的商品 ID")
	}
	if makerName != "" {
		m.Maker = &domain.Maker{Provider: "fc2db", Name: makerName}
	}

	m.Actors = collectFC2DBActors(doc, base)
	m.Tags = collectFC2DBTags(doc, base)

	if cover := fc2dbImage(base, meta("og:image")); cover != "" {
		m.Cover = cover
		m.Thumbnail = cover
	}
	result := domain.MovieMetadata{Detail: m}
	if m.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "fc2db", URL: m.Cover, Role: "cover", Layout: domain.CoverSingle})
	}
	return result, nil
}

// collectFC2DBActors reads every /actress/{id}/ link. fc2db work pages only link
// the work's own actresses this way (no sidebar actress list), and this does not
// depend on how a theme nests the label and the links.
func collectFC2DBActors(doc *html.Node, base string) []domain.Actor {
	var actors []domain.Actor
	seen := make(map[string]bool)
	for _, a := range nodes(doc, func(n *html.Node) bool { return n.Data == "a" }) {
		name := text(a)
		parts := fc2dbActress.FindStringSubmatch(pathOf(absolute(base, attr(a, "href"))))
		if parts == nil || name == "" || seen[parts[1]] {
			continue
		}
		seen[parts[1]] = true
		actors = append(actors, domain.Actor{Provider: "fc2db", ID: parts[1], Name: name})
	}
	return actors
}

// collectFC2DBTags reads every /work-tags/<slug>/ link. url.Parse already
// percent-decodes the path, so the slug needs no further unescaping.
func collectFC2DBTags(doc *html.Node, base string) []domain.Tag {
	var tags []domain.Tag
	seen := make(map[string]bool)
	for _, a := range nodes(doc, func(n *html.Node) bool { return n.Data == "a" }) {
		name := text(a)
		parts := fc2dbTagPath.FindStringSubmatch(pathOf(absolute(base, attr(a, "href"))))
		if parts == nil || name == "" || seen[parts[1]] {
			continue
		}
		seen[parts[1]] = true
		tags = append(tags, domain.Tag{Provider: "fc2db", ID: parts[1], Name: name})
	}
	return tags
}

func stripBracketPrefix(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "[") {
		if i := strings.Index(raw, "]"); i >= 0 {
			return strings.TrimSpace(raw[i+1:])
		}
	}
	return raw
}

func pathOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Path
}

func fc2dbHost(host string) bool {
	return host == "fc2db.net" || strings.HasSuffix(host, ".fc2db.net")
}

func fc2dbImage(base, ref string) string {
	if ref == "" {
		return ""
	}
	u, err := url.Parse(absolute(base, ref))
	if err != nil || u.Scheme != "https" || u.User != nil || !fc2dbHost(u.Hostname()) {
		return ""
	}
	return u.String()
}
