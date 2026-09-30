package providers

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

type MGStage struct{ client }

func NewMGStage(proxy *netx.ProxyManager) *MGStage {
	return &MGStage{newClient(proxy, "https://www.mgstage.com/", "adc=1", "mgstage.com", "mgstage.jp")}
}
func (*MGStage) ID() string { return "mgstage" }

var catalogueCode = regexp.MustCompile(`^\d*[A-Z]+-\d+[A-Z]?$`)

func (*MGStage) Supports(code string) bool {
	return catalogueCode.MatchString(code) && !strings.HasPrefix(code, "FC2-") && !strings.HasPrefix(code, "HEYZO-")
}
func (s *MGStage) Fetch(ctx context.Context, code string) (domain.MovieMetadata, error) {
	body, err := s.get(ctx, s.base+"product/product_detail/"+url.PathEscape(code)+"/")
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return parseMGStage(string(body), s.base)
}
func parseMGStage(body, base string) (domain.MovieMetadata, error) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	m := domain.MovieDetail{Movie: domain.Movie{Title: text(tag(id(doc, "center_column"), "h1")), Summary: meta(doc, "og:description")}}
	for _, row := range nodes(class(doc, "detail_data"), func(n *html.Node) bool { return n.Data == "tr" }) {
		value := tag(row, "td")
		switch strings.TrimSpace(text(tag(row, "th"))) {
		case "品番：":
			m.Code = codeid.Normalize(text(value))
		case "出演：":
			for _, a := range nodes(value, func(n *html.Node) bool { return n.Data == "a" }) {
				u, _ := url.Parse(attr(a, "href"))
				sourceID := ""
				if u != nil {
					sourceID = u.Query().Get("actor[]")
					if sourceID == "" {
						sourceID = u.Query().Get("actor")
					}
				}
				m.Actors = append(m.Actors, domain.Actor{Provider: "mgstage", ID: sourceID, Name: text(a)})
			}
		case "メーカー：":
			m.Maker = &domain.Maker{Provider: "mgstage", Name: text(value)}
		case "シリーズ：":
			m.Series = &domain.Series{Provider: "mgstage", Name: text(value)}
		case "収録時間：":
			m.Duration = integer(text(value))
		case "配信開始日：", "商品発売日：":
			if m.ReleaseDate == "" {
				m.ReleaseDate = releaseDate(text(value))
			}
		case "ジャンル：":
			for _, a := range nodes(value, func(n *html.Node) bool { return n.Data == "a" }) {
				m.Tags = append(m.Tags, domain.Tag{Provider: "mgstage", ID: path.Base(attr(a, "href")), Name: text(a)})
			}
		}
	}
	if m.Code == "" || m.Title == "" {
		return domain.MovieMetadata{}, fmt.Errorf("MGStage 商品页缺少标题或品番")
	}
	m.Sources = []domain.SourceID{{Provider: "mgstage", ID: m.Code}}
	m.Cover = absolute(base, attr(id(doc, "EnlargeImage"), "href"))
	m.Thumbnail = m.Cover
	result := domain.MovieMetadata{Detail: m}
	if m.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "mgstage", URL: m.Cover, Role: "cover"})
	}
	for _, a := range nodes(id(doc, "sample-photo"), func(n *html.Node) bool { return n.Data == "a" }) {
		if href := absolute(base, attr(a, "href")); href != "" {
			result.Images = append(result.Images, domain.ImageCandidate{Provider: "mgstage", URL: href, Role: "preview"})
		}
	}
	return result, nil
}

var _ metadata.Source = (*MGStage)(nil)
