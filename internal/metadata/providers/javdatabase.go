package providers

import (
	"context"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

const javDatabaseBase = "https://www.javdatabase.com/"

// JavDatabase adapts JavDatabase.com movie pages into the metadata provider contract.
type JavDatabase struct {
	client client
}

func NewJavDatabase(proxy *netx.ProxyManager) *JavDatabase {
	return &JavDatabase{client: newClient(proxy, javDatabaseBase, "", "javdatabase.com", "dmm.co.jp")}
}

func (*JavDatabase) ID() string { return "javdatabase" }

func (*JavDatabase) Supports(code string) bool { return strings.TrimSpace(code) != "" }

func (s *JavDatabase) Media(ctx context.Context, target string) (domain.Media, error) {
	return s.client.Media(ctx, target)
}

func (s *JavDatabase) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	if code == "" {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	slug := strings.ToLower(code)
	body, err := s.client.get(ctx, javDatabaseBase+"movies/"+slug+"/")
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return parseJavDatabaseHTML(body, code)
}

func parseJavDatabaseHTML(body []byte, code string) (domain.MovieMetadata, error) {
	root, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	dvdID := javDatabaseText(root, "DVD ID:")
	if dvdID == "" || !codeid.IsFormatEquivalent(dvdID, code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	title := strings.TrimSpace(strings.TrimPrefix(text(tag(root, "h1")), code+" - "))
	movie := domain.Movie{
		Code:        code,
		Title:       title,
		ReleaseDate: releaseDate(javDatabaseText(root, "Release Date:")),
		Duration:    integer(javDatabaseText(root, "Runtime:")),
		Cover:       javDatabaseCover(root),
		Sources:     []domain.SourceID{{Provider: "javdatabase", ID: dvdID}},
	}
	movie.Thumbnail = movie.Cover
	if name, href := javDatabaseLink(root, "Studio:"); name != "" {
		movie.Maker = &domain.Maker{Provider: "javdatabase", ID: javDatabaseID(href), Name: name}
	}
	if name, href := javDatabaseLink(root, "JAV Series:"); name != "" {
		movie.Series = &domain.Series{Provider: "javdatabase", ID: javDatabaseID(href), Name: name}
	}
	if name, href := javDatabaseLink(root, "Director:"); name != "" {
		movie.Director = &domain.Director{Provider: "javdatabase", ID: javDatabaseID(href), Name: name}
	}
	for _, link := range javDatabaseLinks(root, "Genre(s):") {
		name := text(link)
		if name != "" {
			movie.Tags = append(movie.Tags, domain.Tag{Provider: "javdatabase", ID: javDatabaseID(attr(link, "href")), Name: name})
		}
	}
	for _, link := range javDatabaseLinks(root, "Idol(s)/Actress(es):") {
		name := text(link)
		if name != "" {
			movie.Actors = append(movie.Actors, domain.Actor{Provider: "javdatabase", ID: javDatabaseID(attr(link, "href")), Name: name})
		}
	}

	result := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: movie}}
	if movie.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "javdatabase", URL: movie.Cover, Role: "cover"})
	}
	return result, nil
}

func javDatabaseParagraph(root *html.Node, label string) *html.Node {
	for _, paragraph := range nodes(root, func(n *html.Node) bool {
		return n.Data == "p" && hasClass(n, "mb-1")
	}) {
		heading := tag(paragraph, "b")
		if heading != nil && strings.HasPrefix(strings.TrimSpace(text(heading)), label) {
			return paragraph
		}
	}
	return nil
}

func javDatabaseText(root *html.Node, label string) string {
	paragraph := javDatabaseParagraph(root, label)
	if paragraph == nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(text(paragraph), text(tag(paragraph, "b"))))
}

func javDatabaseLinks(root *html.Node, label string) []*html.Node {
	paragraph := javDatabaseParagraph(root, label)
	if paragraph == nil {
		return nil
	}
	return nodes(paragraph, func(n *html.Node) bool { return n.Data == "a" })
}

func javDatabaseLink(root *html.Node, label string) (string, string) {
	if links := javDatabaseLinks(root, label); len(links) > 0 {
		return text(links[0]), attr(links[0], "href")
	}
	return "", ""
}

func javDatabaseCover(root *html.Node) string {
	for _, meta := range nodes(root, func(n *html.Node) bool {
		return n.Data == "meta" && attr(n, "property") == "og:image"
	}) {
		if url := attr(meta, "content"); url != "" {
			return url
		}
	}
	return ""
}

func javDatabaseID(href string) string {
	href = strings.TrimSuffix(strings.TrimSpace(href), "/")
	if index := strings.LastIndex(href, "/"); index >= 0 {
		return href[index+1:]
	}
	return href
}
