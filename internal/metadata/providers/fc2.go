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

type FC2 struct{ client }

func NewFC2(proxy *netx.ProxyManager) *FC2 {
	return &FC2{newClient(proxy, "https://adult.contents.fc2.com/", "", "fc2.com")}
}
func (*FC2) ID() string { return "fc2" }

var fc2Code = regexp.MustCompile(`^FC2-(?:PPV-)?(\d+)$`)
var fc2ProductID = regexp.MustCompile(`^(?:FC2(?:[- ]*PPV)?[- ]*)?([0-9]+)$`)

func (*FC2) Supports(code string) bool { return fc2Code.MatchString(code) }
func (s *FC2) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	parts := fc2Code.FindStringSubmatch(ref.Code)
	if parts == nil {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	id := parts[1]
	body, err := s.get(ctx, s.base+"article/"+id+"/")
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return parseFC2(string(body), s.base, id)
}
func parseFC2(body, base, productID string) (domain.MovieMetadata, error) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	if class(doc, "items_notfound_header") != nil {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	meta := func(property string) string {
		return attr(first(doc, func(n *html.Node) bool {
			return n.Data == "meta" && attr(n, "property") == property
		}), "content")
	}
	confirmed := false
	if ref := meta("og:url"); ref != "" {
		u, err := url.Parse(fc2MediaURL(base, ref))
		if err != nil || strings.TrimSuffix(u.Path, "/") != "/article/"+productID {
			return domain.MovieMetadata{}, fmt.Errorf("FC2 商品页身份与请求不一致")
		}
		confirmed = true
	}
	header := class(doc, "items_article_headerInfo")
	m := domain.MovieDetail{Movie: domain.Movie{Code: "FC2-" + productID, Title: text(tag(header, "h3")), Sources: []domain.SourceID{{Provider: "fc2", ID: productID}}}, Zone: domain.ZoneFC2}
	if m.Title == "" {
		return domain.MovieMetadata{}, fmt.Errorf("FC2 商品页未返回标题")
	}
	m.OriginTitle = m.Title
	for _, section := range nodes(doc, func(n *html.Node) bool { return hasClass(n, "items_article_softDevice") }) {
		for _, p := range nodes(section, func(n *html.Node) bool { return n.Data == "p" }) {
			key, value, _ := strings.Cut(strings.ReplaceAll(text(p), "：", ":"), ":")
			switch strings.TrimSpace(key) {
			case "Product ID", "商品ID":
				parts := fc2ProductID.FindStringSubmatch(strings.TrimSpace(value))
				if parts == nil || parts[1] != productID {
					return domain.MovieMetadata{}, fmt.Errorf("FC2 商品 ID 与请求不一致")
				}
				confirmed = true
			case "Sale Day", "販売日":
				m.ReleaseDate = releaseDate(value)
			}
		}
	}
	if !confirmed {
		return domain.MovieMetadata{}, fmt.Errorf("FC2 商品页缺少可确认的商品 ID")
	}
	if m.ReleaseDate == "" {
		m.ReleaseDate = releaseDate(text(class(header, "items_article_Releasedate")))
	}
	thumb := class(doc, "items_article_MainitemThumb")
	m.Thumbnail = fc2MediaURL(base, attr(tag(thumb, "img"), "src"))
	m.Cover = fc2MediaURL(base, meta("og:image"))
	if m.Cover == "" {
		m.Cover = m.Thumbnail
	}
	if m.Thumbnail == "" {
		m.Thumbnail = m.Cover
	}
	duration := strings.Split(text(class(thumb, "items_article_info")), ":")
	m.Duration = integer(duration[0])
	if len(duration) == 3 {
		m.Duration = m.Duration*60 + integer(duration[1])
	}
	result := domain.MovieMetadata{Detail: m}
	for _, image := range []string{m.Cover, m.Thumbnail} {
		if image != "" && (len(result.Images) == 0 || result.Images[0].URL != image) {
			result.Images = append(result.Images, domain.ImageCandidate{Provider: "fc2", URL: image, Role: "cover", Layout: domain.CoverSingle})
		}
	}
	seen := make(map[string]bool)
	for _, a := range nodes(class(doc, "items_article_SampleImages"), func(n *html.Node) bool { return n.Data == "a" }) {
		if href := fc2MediaURL(base, attr(a, "href")); href != "" && !seen[href] {
			seen[href] = true
			result.Images = append(result.Images, domain.ImageCandidate{Provider: "fc2", URL: href, Role: "preview"})
			thumbnail := fc2MediaURL(base, attr(tag(a, "img"), "src"))
			if thumbnail == "" {
				thumbnail = href
			}
			result.Detail.PreviewImages = append(result.Detail.PreviewImages, domain.PreviewImage{Original: href, Thumbnail: thumbnail})
		}
	}
	result.Detail.HasPreview = len(result.Detail.PreviewImages) > 0
	return result, nil
}

func fc2MediaURL(base, ref string) string {
	if ref == "" {
		return ""
	}
	u, err := url.Parse(absolute(base, ref))
	if err != nil || u.Scheme != "https" || u.User != nil ||
		(u.Hostname() != "fc2.com" && !strings.HasSuffix(u.Hostname(), ".fc2.com")) {
		return ""
	}
	return u.String()
}
