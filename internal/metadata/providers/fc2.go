package providers

import (
	"context"
	"fmt"
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

func (*FC2) Supports(code string) bool { return fc2Code.MatchString(code) }
func (s *FC2) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	id := fc2Code.FindStringSubmatch(ref.Code)[1]
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
	header := class(doc, "items_article_headerInfo")
	m := domain.MovieDetail{Movie: domain.Movie{Code: "FC2-" + productID, Title: text(tag(header, "h3")), Sources: []domain.SourceID{{Provider: "fc2", ID: productID}}}, Zone: domain.ZoneFC2}
	if m.Title == "" {
		return domain.MovieMetadata{}, fmt.Errorf("FC2 商品页未返回标题")
	}
	for _, p := range nodes(class(header, "items_article_softDevice"), func(n *html.Node) bool { return n.Data == "p" }) {
		key, value, _ := strings.Cut(text(p), ":")
		switch strings.TrimSpace(key) {
		case "Product ID", "商品ID":
			if strings.TrimSpace(value) != productID {
				return domain.MovieMetadata{}, fmt.Errorf("FC2 商品 ID 与请求不一致")
			}
		case "Sale Day", "販売日":
			m.ReleaseDate = releaseDate(value)
		}
	}
	if m.ReleaseDate == "" {
		m.ReleaseDate = releaseDate(text(class(header, "items_article_Releasedate")))
	}
	thumb := class(doc, "items_article_MainitemThumb")
	m.Cover = absolute(base, attr(tag(thumb, "img"), "src"))
	m.Thumbnail = m.Cover
	m.Duration = integer(text(class(thumb, "items_article_info")))
	result := domain.MovieMetadata{Detail: m}
	if m.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "fc2", URL: m.Cover, Role: "cover", Layout: domain.CoverSingle})
	}
	for _, a := range nodes(class(doc, "items_article_SampleImages"), func(n *html.Node) bool { return n.Data == "a" }) {
		if href := absolute(base, attr(a, "href")); href != "" {
			result.Images = append(result.Images, domain.ImageCandidate{Provider: "fc2", URL: href, Role: "preview"})
		}
	}
	return result, nil
}
