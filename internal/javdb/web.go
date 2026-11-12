package javdb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	http "github.com/bogdanfinn/fhttp"
	"github.com/ppxb/miyabi/internal/domain"
	"golang.org/x/net/html"
)

// JavDB gates its anonymous App API for search and detail. Movie detail is
// served from the public web pages; search requires a configured account.
const (
	webUserAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	webMaxBody    = 4 << 20
	webAcceptHead = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
)

// webBaseURL is the public site that serves the anonymous pages.
var webBaseURL = "https://javdb.com"

var (
	webDurationPattern = regexp.MustCompile(`(\d+)`)
	webRatingPattern   = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*分`)
	webDatePattern     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// requiresWebFallback reports whether the API refused the request in a way the
// public web pages can serve (missing token or a challenge-style rejection).
func requiresWebFallback(err error) bool {
	var api *APIError
	if errors.As(err, &api) {
		return strings.EqualFold(api.Action, "JWTVerificationError")
	}
	var response *HTTPError
	if errors.As(err, &response) {
		return response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden
	}
	return false
}

// fetchWebHTML downloads one public page through the fingerprint client so it
// keeps the configured proxy and browser TLS profile.
func (c *Client) fetchWebHTML(ctx context.Context, path string, params url.Values) ([]byte, error) {
	if c.fingerprint == nil {
		return nil, errors.New("JavDB web client is unavailable")
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	endpoint := webBaseURL + path
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create JavDB web request: %w", err)
	}
	request.Header.Set("user-agent", webUserAgent)
	request.Header.Set("accept", webAcceptHead)
	request.Header.Set("accept-language", defaultLanguage)

	response, err := c.fingerprint.Do(request)
	if err != nil {
		return nil, &networkError{err: err}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.CopyN(io.Discard, response.Body, 4096)
		return nil, &HTTPError{StatusCode: response.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, webMaxBody+1))
	if err != nil {
		return nil, &networkError{err: fmt.Errorf("read JavDB web response: %w", err)}
	}
	if len(body) > webMaxBody {
		return nil, &networkError{err: fmt.Errorf("JavDB web response exceeded %d bytes", webMaxBody)}
	}
	if bytes.Contains(body, []byte("Just a moment...")) || bytes.Contains(body, []byte("Attention Required! | Cloudflare")) {
		return nil, &networkError{err: errors.New("JavDB web challenge encountered")}
	}
	return body, nil
}

func (c *Client) movieDetailFromWeb(ctx context.Context, movieID string) (domain.MovieDetail, error) {
	body, err := c.fetchWebHTML(ctx, "/v/"+url.PathEscape(movieID), nil)
	if err != nil {
		return domain.MovieDetail{}, err
	}
	detail, err := parseWebMovieDetail(body, movieID)
	if err != nil {
		return domain.MovieDetail{}, err
	}
	if detail.Code == "" || detail.Title == "" {
		return domain.MovieDetail{}, errors.New("JavDB web page has no movie details")
	}
	return detail, nil
}

func parseWebMovieDetail(raw []byte, movieID string) (domain.MovieDetail, error) {
	document, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return domain.MovieDetail{}, err
	}
	movie := domain.Movie{
		Sources:      []domain.SourceID{{Provider: "javdb", ID: movieID}},
		RatingSource: "javdb", RatingMax: 5,
		ID: movieID,
	}
	detail := domain.MovieDetail{Movie: movie, Zone: domain.ZoneUnknown}

	heading := htmlFind(document, func(n *html.Node) bool {
		return htmlIsElement(n, "h2") && htmlHasClass(n, "title")
	})
	if heading != nil {
		if node := htmlFind(heading, func(n *html.Node) bool {
			return htmlIsElement(n, "strong") && htmlHasClass(n, "current-title")
		}); node != nil {
			detail.Title = strings.TrimSpace(htmlText(node))
		}
		if node := htmlFind(heading, func(n *html.Node) bool {
			return htmlIsElement(n, "span") && htmlHasClass(n, "origin-title")
		}); node != nil {
			detail.OriginTitle = strings.TrimSpace(htmlText(node))
		}
	}
	if detail.Title == "" {
		detail.Title = detail.OriginTitle
	}
	if cover := htmlFind(document, func(n *html.Node) bool {
		return htmlIsElement(n, "img") && htmlHasClass(n, "video-cover")
	}); cover != nil {
		detail.Cover = htmlAttr(cover, "src")
		detail.Thumbnail = detail.Cover
	}

	for _, block := range htmlFindAll(document, func(n *html.Node) bool {
		return htmlIsElement(n, "div") && htmlHasClass(n, "panel-block")
	}) {
		label := htmlFind(block, func(n *html.Node) bool { return htmlIsElement(n, "strong") })
		if label == nil {
			continue
		}
		key := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(htmlText(label)), ":"))
		value := htmlFind(block, func(n *html.Node) bool {
			return htmlIsElement(n, "span") && htmlHasClass(n, "value")
		})
		if value == nil {
			value = block
		}
		switch key {
		case "番號", "番号":
			if detail.Code == "" {
				detail.Code = strings.Join(strings.Fields(htmlText(value)), "")
			}
		case "日期":
			if date := strings.TrimSpace(htmlText(value)); webDatePattern.MatchString(date) {
				detail.ReleaseDate = date
			}
		case "時長", "时长":
			if match := webDurationPattern.FindStringSubmatch(htmlText(value)); len(match) == 2 {
				detail.Duration, _ = strconv.Atoi(match[1])
			}
		case "評分", "评分":
			detail.Rating = webRating(htmlText(value))
		case "導演", "导演":
			if id, name := webEntityRef(value, "/directors/"); id != "" {
				detail.Director = &domain.Director{Provider: "javdb", ID: id, Name: name}
			}
		case "片商", "發行商", "发行商":
			if id, name := webEntityRef(value, "/makers/"); id != "" {
				detail.Maker = &domain.Maker{Provider: "javdb", ID: id, Name: name}
			}
		case "系列":
			if id, name := webEntityRef(value, "/series/"); id != "" {
				detail.Series = &domain.Series{Provider: "javdb", ID: id, Name: name}
			}
		case "類別", "类别":
			detail.Tags = webTags(value)
		case "演員", "演员":
			detail.Actors = webActors(value)
		}
	}

	if detail.Title == "" && heading != nil {
		detail.Title = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(htmlText(heading)), detail.Code))
	}
	detail.PreviewImages = webPreviewImages(document)
	detail.HasPreview = len(detail.PreviewImages) > 0
	detail.ActorMovies = webRecommendations(document, "出演")
	detail.RelatedMovies = webRecommendations(document, "喜歡", "喜欢")
	return detail, nil
}

func webMovieID(href string) string {
	parsed, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(parsed.Path, "/v/")
}

func webRating(text string) float64 {
	if match := webRatingPattern.FindStringSubmatch(text); len(match) == 2 {
		value, _ := strconv.ParseFloat(match[1], 64)
		return value
	}
	return 0
}

func webEntityRef(node *html.Node, prefix string) (id, name string) {
	link := htmlFind(node, func(n *html.Node) bool {
		return htmlIsElement(n, "a") && strings.Contains(htmlAttr(n, "href"), prefix)
	})
	if link == nil {
		return "", ""
	}
	return webEntityID(htmlAttr(link, "href"), prefix), strings.TrimSpace(htmlText(link))
}

func webEntityID(href, prefix string) string {
	index := strings.Index(href, prefix)
	if index < 0 {
		return ""
	}
	rest := href[index+len(prefix):]
	if cut := strings.IndexAny(rest, "?#"); cut >= 0 {
		rest = rest[:cut]
	}
	return strings.TrimSuffix(rest, "/")
}

func webTags(node *html.Node) []domain.Tag {
	tags := make([]domain.Tag, 0)
	for _, link := range htmlFindAll(node, func(n *html.Node) bool {
		return htmlIsElement(n, "a") && strings.Contains(htmlAttr(n, "href"), "/tags")
	}) {
		name := strings.TrimSpace(htmlText(link))
		if name != "" {
			tags = append(tags, domain.Tag{Provider: "javdb", Name: name, NameZHT: name})
		}
	}
	return tags
}

func webActors(node *html.Node) []domain.Actor {
	actors := make([]domain.Actor, 0)
	for _, link := range htmlFindAll(node, func(n *html.Node) bool {
		return htmlIsElement(n, "a") && strings.Contains(htmlAttr(n, "href"), "/actors/")
	}) {
		name := strings.TrimSpace(htmlText(link))
		id := webEntityID(htmlAttr(link, "href"), "/actors/")
		if name == "" || id == "" {
			continue
		}
		gender := "unknown"
		if htmlHasClass(link, "actor-female") {
			gender = "female"
		}
		actors = append(actors, domain.Actor{Provider: "javdb", ID: id, Name: name, NameZHT: name, Gender: gender})
	}
	return actors
}

func webPreviewImages(document *html.Node) []domain.PreviewImage {
	gallery := htmlFind(document, func(n *html.Node) bool {
		return htmlIsElement(n, "div") && htmlHasClass(n, "preview-images")
	})
	if gallery == nil {
		return []domain.PreviewImage{}
	}
	images := make([]domain.PreviewImage, 0)
	for _, link := range htmlFindAll(gallery, func(n *html.Node) bool {
		return htmlIsElement(n, "a") && htmlHasClass(n, "tile-item")
	}) {
		original := htmlAttr(link, "href")
		if !strings.HasPrefix(original, "https://") {
			continue
		}
		thumbnail := ""
		if img := htmlFind(link, func(n *html.Node) bool { return htmlIsElement(n, "img") }); img != nil {
			thumbnail = htmlAttr(img, "src")
		}
		images = append(images, domain.PreviewImage{Thumbnail: thumbnail, Original: original})
	}
	return images
}

// webRecommendations reads a recommendation panel by the marker in its header.
// The header text differs by language, so callers pass the matching fragment.
func webRecommendations(document *html.Node, markers ...string) []domain.MovieReference {
	for _, article := range htmlFindAll(document, func(n *html.Node) bool {
		return htmlIsElement(n, "article") && htmlHasClass(n, "video-panel")
	}) {
		header := htmlFind(article, func(n *html.Node) bool {
			return htmlIsElement(n, "div") && htmlHasClass(n, "message-header")
		})
		if header == nil || !webContainsAny(htmlText(header), markers) {
			continue
		}
		references := make([]domain.MovieReference, 0)
		for _, link := range htmlFindAll(article, func(n *html.Node) bool {
			return htmlIsElement(n, "a") && htmlHasClass(n, "tile-item")
		}) {
			id := webMovieID(htmlAttr(link, "href"))
			code := ""
			if node := htmlFind(link, func(n *html.Node) bool {
				return htmlIsElement(n, "div") && htmlHasClass(n, "video-number")
			}); node != nil {
				code = strings.TrimSpace(htmlText(node))
			}
			thumbnail := ""
			if img := htmlFind(link, func(n *html.Node) bool { return htmlIsElement(n, "img") }); img != nil {
				thumbnail = htmlAttr(img, "src")
			}
			if id == "" || code == "" {
				continue
			}
			references = append(references, domain.MovieReference{ID: id, Code: code, Thumbnail: thumbnail})
		}
		return references
	}
	return []domain.MovieReference{}
}

func webContainsAny(text string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func htmlIsElement(node *html.Node, tag string) bool {
	return node.Type == html.ElementNode && strings.EqualFold(node.Data, tag)
}

func htmlHasClass(node *html.Node, class string) bool {
	for _, field := range strings.Fields(htmlAttr(node, "class")) {
		if field == class {
			return true
		}
	}
	return false
}

func htmlAttr(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func htmlText(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return builder.String()
}

func htmlFind(node *html.Node, match func(*html.Node) bool) *html.Node {
	if node == nil {
		return nil
	}
	if match(node) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := htmlFind(child, match); found != nil {
			return found
		}
	}
	return nil
}

func htmlFindAll(node *html.Node, match func(*html.Node) bool) []*html.Node {
	var found []*html.Node
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if match(current) {
			found = append(found, current)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return found
}
