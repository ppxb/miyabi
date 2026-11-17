package providers

import (
	"context"
	"fmt"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

const javbusBase = "https://www.javbus.com/"

// JavBusClient is the page fetch the JavBus metadata provider depends on.
type JavBusClient interface {
	MoviePage(context.Context, string) (string, error)
}

// JavBus adapts the JavBus movie page into the metadata provider contract.
type JavBus struct {
	client JavBusClient
	media  client
}

func NewJavBus(fetcher JavBusClient, proxy *netx.ProxyManager) *JavBus {
	adapter := &JavBus{client: fetcher}
	if proxy != nil {
		adapter.media = newClient(proxy, javbusBase, "", "javbus.com")
	}
	return adapter
}

func (*JavBus) ID() string { return "javbus" }

func (*JavBus) Supports(code string) bool {
	upper := strings.ToUpper(code)
	return upper != "" && !strings.HasPrefix(upper, "FC2") && !strings.HasPrefix(upper, "HEYZO")
}

func (s *JavBus) Media(ctx context.Context, target string) (domain.Media, error) {
	return s.media.Media(ctx, target)
}

func (s *JavBus) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	if code == "" || !s.Supports(code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	body, err := s.client.MoviePage(ctx, code)
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	if body == "" {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return domain.MovieMetadata{}, fmt.Errorf("parse JavBus page: %w", err)
	}
	info := class(root, "info")
	if info == nil {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	if got := codeid.Normalize(javbusField(info, "識別碼")); got != code {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}

	title := text(tag(root, "h3"))
	title = strings.TrimSpace(strings.TrimPrefix(title, code))

	movie := domain.Movie{
		Code:        code,
		Title:       title,
		ReleaseDate: releaseDate(javbusField(info, "發行日期")),
		Duration:    parseDuration(javbusField(info, "長度")),
		Cover:       javbusCover(root),
		Tags:        javbusTags(info),
		Actors:      javbusActors(root),
		Sources:     []domain.SourceID{{Provider: "javbus", ID: code}},
	}
	movie.Thumbnail = movie.Cover
	if name := javbusLink(info, "導演"); name != "" {
		movie.Director = &domain.Director{Provider: "javbus", ID: name, Name: name}
	}
	if name := javbusLink(info, "製作商"); name != "" {
		movie.Maker = &domain.Maker{Provider: "javbus", ID: name, Name: name}
	}
	if name := javbusLink(info, "系列"); name != "" {
		movie.Series = &domain.Series{Provider: "javbus", ID: name, Name: name}
	}

	result := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: movie}}
	if movie.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "javbus", URL: movie.Cover, Role: "cover"})
	}
	return result, nil
}

func javbusField(info *html.Node, label string) string {
	for _, paragraph := range nodes(info, func(n *html.Node) bool { return n.Data == "p" }) {
		header := class(paragraph, "header")
		if header == nil {
			continue
		}
		headerText := text(header)
		if !strings.Contains(headerText, label) {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(text(paragraph), headerText))
	}
	return ""
}

func javbusLink(info *html.Node, label string) string {
	for _, paragraph := range nodes(info, func(n *html.Node) bool { return n.Data == "p" }) {
		header := class(paragraph, "header")
		if header == nil || !strings.Contains(text(header), label) {
			continue
		}
		if link := first(paragraph, func(n *html.Node) bool { return n.Data == "a" }); link != nil {
			return text(link)
		}
	}
	return ""
}

func javbusCover(root *html.Node) string {
	for _, anchor := range nodes(root, func(n *html.Node) bool { return hasClass(n, "bigImage") }) {
		if image := first(anchor, func(n *html.Node) bool { return n.Data == "img" }); image != nil {
			return absolute(javbusBase, attr(image, "src"))
		}
	}
	return ""
}

func javbusTags(info *html.Node) []domain.Tag {
	seen := make(map[string]bool)
	var tags []domain.Tag
	for _, span := range nodes(info, func(n *html.Node) bool { return hasClass(n, "genre") }) {
		link := first(span, func(n *html.Node) bool { return n.Data == "a" })
		if link == nil {
			continue
		}
		name := text(link)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		tags = append(tags, domain.Tag{Provider: "javbus", ID: name, Name: name})
	}
	return tags
}

func javbusActors(root *html.Node) []domain.Actor {
	seen := make(map[string]bool)
	var actors []domain.Actor
	for _, name := range nodes(root, func(n *html.Node) bool { return hasClass(n, "star-name") }) {
		link := first(name, func(n *html.Node) bool { return n.Data == "a" })
		if link == nil {
			continue
		}
		actorName := attr(link, "title")
		if actorName == "" {
			actorName = text(link)
		}
		href := attr(link, "href")
		id := href[strings.LastIndex(href, "/")+1:]
		if actorName == "" || seen[id] {
			continue
		}
		seen[id] = true
		actors = append(actors, domain.Actor{Provider: "javbus", ID: id, Name: actorName})
	}
	return actors
}

func parseDuration(value string) int {
	return integer(value)
}
