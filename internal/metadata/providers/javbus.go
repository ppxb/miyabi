package providers

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/javbus"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

type JavBus struct {
	client
	pages *javbus.Client
}

// The composition root shares this page client with magnet retrieval.
func NewJavBus(pages *javbus.Client, proxy *netx.ProxyManager) *JavBus {
	return &JavBus{client: newClient(proxy, "https://www.javbus.com/", "", "javbus.com", "javbus.org", "javcdn.com", "dmm.co.jp"), pages: pages}
}
func (*JavBus) ID() string                { return "javbus" }
func (*JavBus) Supports(code string) bool { return !strings.HasPrefix(code, "FC2-") }
func (s *JavBus) Fetch(ctx context.Context, code string) (domain.MovieMetadata, error) {
	body, err := s.pages.MoviePage(ctx, code)
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	if body == "" {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	return parseJavBus(body, s.base)
}
func parseJavBus(body, base string) (domain.MovieMetadata, error) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	container := class(doc, "container")
	m := domain.MovieDetail{Movie: domain.Movie{Title: text(tag(container, "h3"))}}
	info := class(doc, "info")
	for _, p := range nodes(info, func(n *html.Node) bool { return n.Data == "p" }) {
		label := text(class(p, "header"))
		value := strings.TrimSpace(strings.TrimPrefix(text(p), label))
		switch label {
		case "識別碼:", "识别码:":
			m.Code = codeid.Normalize(value)
		case "發行日期:", "发行日期:":
			m.ReleaseDate = releaseDate(value)
		case "長度:", "长度:":
			m.Duration = integer(value)
		case "製作商:", "制作商:":
			m.Maker = &domain.Maker{Provider: "javbus", ID: path.Base(attr(tag(p, "a"), "href")), Name: text(tag(p, "a"))}
		case "導演:", "导演:":
			m.Director = &domain.Director{Provider: "javbus", ID: path.Base(attr(tag(p, "a"), "href")), Name: text(tag(p, "a"))}
		case "系列:":
			m.Series = &domain.Series{Provider: "javbus", ID: path.Base(attr(tag(p, "a"), "href")), Name: text(tag(p, "a"))}
		}
	}
	if m.Code == "" || m.Title == "" {
		return domain.MovieMetadata{}, fmt.Errorf("JavBus 详情页缺少标题或番号")
	}
	m.Title = strings.TrimSpace(strings.TrimPrefix(m.Title, m.Code))
	m.Sources = []domain.SourceID{{Provider: "javbus", ID: m.Code}}
	m.Cover = absolute(base, attr(class(doc, "bigImage"), "href"))
	m.Thumbnail = m.Cover
	for _, a := range nodes(info, func(n *html.Node) bool { return n.Data == "a" }) {
		u, err := url.Parse(attr(a, "href"))
		if err != nil {
			continue
		}
		if strings.HasPrefix(u.Path, "/genre/") {
			m.Tags = append(m.Tags, domain.Tag{Provider: "javbus", ID: path.Base(u.Path), Name: text(a)})
		}
	}
	for _, a := range nodes(doc, func(n *html.Node) bool { return n.Data == "a" && hasClass(n, "avatar-box") }) {
		m.Actors = append(m.Actors, domain.Actor{Provider: "javbus", ID: path.Base(attr(a, "href")), Name: text(tag(a, "span")), Avatar: absolute(base, attr(tag(a, "img"), "src"))})
	}
	result := domain.MovieMetadata{Detail: m}
	if m.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "javbus", URL: m.Cover, Role: "cover"})
	}
	for _, a := range nodes(doc, func(n *html.Node) bool { return n.Data == "a" && hasClass(n, "sample-box") }) {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "javbus", URL: absolute(base, attr(a, "href")), Role: "preview"})
	}
	return result, nil
}
