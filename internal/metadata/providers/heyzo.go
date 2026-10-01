package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

type HEYZO struct{ client }

func NewHEYZO(proxy *netx.ProxyManager) *HEYZO {
	return &HEYZO{newClient(proxy, "https://www.heyzo.com/", "", "heyzo.com")}
}

func (*HEYZO) ID() string { return "heyzo" }

var (
	heyzoCode     = regexp.MustCompile(`^HEYZO-([0-9]+)$`)
	heyzoEntity   = regexp.MustCompile(`^/listpages/(actor|series|category)_([0-9]+)_[0-9]+\.html$`)
	heyzoKeyword  = regexp.MustCompile(`^/search/([^/]+)/[0-9]+\.html$`)
	heyzoImage    = regexp.MustCompile(`^/contents/[0-9]+/([0-9]+)/(images/[^/]+|gallery/[0-9]+)\.(?:jpg|jpeg|png|webp)$`)
	heyzoHref     = regexp.MustCompile(`\bhref\s*=\s*["']([^"']+)["']`)
	heyzoDuration = regexp.MustCompile(`^PT(?:([0-9]+)H)?(?:([0-9]+)M)?(?:([0-9]+)S)?$`)
)

func (*HEYZO) Supports(code string) bool { return heyzoCode.MatchString(code) }

func (s *HEYZO) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	parts := heyzoCode.FindStringSubmatch(ref.Code)
	if parts == nil {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	body, err := s.get(ctx, s.base+"moviepages/"+parts[1]+"/index.html")
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return parseHEYZO(string(body), s.base, parts[1])
}

func parseHEYZO(body, base, productID string) (domain.MovieMetadata, error) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	meta := func(property string) string {
		return attr(first(doc, func(n *html.Node) bool {
			return n.Data == "meta" && attr(n, "property") == property
		}), "content")
	}
	identity := heyzoURL(base, meta("og:url"))
	if identity == nil || identity.Path != "/moviepages/"+productID+"/index.html" {
		return domain.MovieMetadata{}, fmt.Errorf("HEYZO 页面身份与请求不一致")
	}
	movie := first(doc, func(n *html.Node) bool { return attr(n, "id") == "movie" })
	title := text(tag(movie, "h1"))
	if title == "" {
		return domain.MovieMetadata{}, fmt.Errorf("HEYZO 影片页未返回标题")
	}
	m := domain.MovieDetail{Movie: domain.Movie{
		Code: "HEYZO-" + productID, Title: title, OriginTitle: title,
		Sources: []domain.SourceID{{Provider: "heyzo", ID: productID}},
		Maker:   &domain.Maker{Provider: "heyzo", ID: "heyzo", Name: "HEYZO"},
	}, Zone: domain.ZoneUncensored}
	info := class(doc, "movieInfo")
	m.ReleaseDate = releaseDate(text(class(info, "table-release-day")))
	for _, script := range nodes(movie, func(n *html.Node) bool {
		return n.Data == "script" && attr(n, "type") == "application/ld+json"
	}) {
		var data struct {
			Type        string `json:"@type"`
			Duration    string `json:"duration"`
			DateCreated string `json:"dateCreated"`
		}
		if script.FirstChild == nil || json.Unmarshal([]byte(script.FirstChild.Data), &data) != nil || data.Type != "Movie" {
			continue
		}
		if parts := heyzoDuration.FindStringSubmatch(data.Duration); parts != nil {
			m.Duration = integer(parts[1])*60 + integer(parts[2]) + integer(parts[3])/60
		}
		if m.ReleaseDate == "" {
			m.ReleaseDate = releaseDate(data.DateCreated)
		}
		break
	}
	tags, actors := make(map[string]bool), make(map[string]bool)
	for _, a := range nodes(info, func(n *html.Node) bool { return n.Data == "a" }) {
		u, name := heyzoURL(base, attr(a, "href")), text(a)
		if u == nil || name == "" {
			continue
		}
		var tagID string
		if parts := heyzoEntity.FindStringSubmatch(u.Path); parts != nil {
			switch parts[1] {
			case "actor":
				if !actors[parts[2]] {
					m.Actors = append(m.Actors, domain.Actor{Provider: "heyzo", ID: parts[2], Name: name})
					actors[parts[2]] = true
				}
			case "series":
				m.Series = &domain.Series{Provider: "heyzo", ID: parts[2], Name: name}
			case "category":
				tagID = "category:" + parts[2]
			}
		} else if parts := heyzoKeyword.FindStringSubmatch(u.Path); parts != nil {
			tagID = "keyword:" + parts[1]
		}
		if tagID != "" && !tags[tagID] {
			m.Tags = append(m.Tags, domain.Tag{Provider: "heyzo", ID: tagID, Name: name})
			tags[tagID] = true
		}
	}
	rating := first(class(info, "table-estimate"), func(n *html.Node) bool { return attr(n, "itemprop") == "ratingValue" })
	if value, err := strconv.ParseFloat(text(rating), 64); err == nil && value > 0 && value <= 5 {
		m.Rating, m.RatingMax, m.RatingSource = value, 5, "heyzo"
	}
	m.Cover = heyzoImageURL(base, meta("og:image"), productID, "images/")
	m.Thumbnail = m.Cover
	result := domain.MovieMetadata{Detail: m}
	if m.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "heyzo", URL: m.Cover, Role: "cover", Layout: domain.CoverSingle})
	}
	gallery := first(doc, func(n *html.Node) bool { return attr(n, "id") == "section_gallery" })
	var galleryHTML strings.Builder
	if gallery != nil {
		if err := html.Render(&galleryHTML, gallery); err != nil {
			return domain.MovieMetadata{}, err
		}
	}
	// Public gallery links are also emitted by document.write. Read their literal
	// hrefs without executing scripts or inventing originals for member thumbnails.
	seen := make(map[string]bool)
	for _, match := range heyzoHref.FindAllStringSubmatch(galleryHTML.String(), -1) {
		u := heyzoImageURL(base, match[1], productID, "gallery/")
		if u != "" && !seen[u] {
			seen[u] = true
			result.Images = append(result.Images, domain.ImageCandidate{Provider: "heyzo", URL: u, Role: "preview"})
			result.Detail.PreviewImages = append(result.Detail.PreviewImages, domain.PreviewImage{Original: u, Thumbnail: u})
		}
	}
	result.Detail.HasPreview = len(result.Detail.PreviewImages) > 0
	return result, nil
}

func heyzoURL(base, ref string) *url.URL {
	if ref == "" {
		return nil
	}
	u, err := url.Parse(absolute(base, ref))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" ||
		(u.Hostname() != "heyzo.com" && !strings.HasSuffix(u.Hostname(), ".heyzo.com")) {
		return nil
	}
	return u
}

func heyzoImageURL(base, ref, productID, folder string) string {
	u := heyzoURL(base, ref)
	if u == nil {
		return ""
	}
	parts := heyzoImage.FindStringSubmatch(u.Path)
	if parts == nil || parts[1] != productID || !strings.HasPrefix(parts[2], folder) {
		return ""
	}
	return u.String()
}
