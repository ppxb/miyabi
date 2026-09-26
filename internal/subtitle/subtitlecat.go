package subtitle

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

const defaultSubtitleCatOrigin = "https://www.subtitlecat.com"

type SubtitleCatProvider struct {
	origin string
	client *resty.Client
}

func NewSubtitleCatProvider(proxyManager *netx.ProxyManager) *SubtitleCatProvider {
	var client *resty.Client
	opts := netx.RestyOptions{Timeout: 15 * time.Second}
	if proxyManager != nil {
		client = netx.NewRestyClient(proxyManager, opts)
	} else {
		client = netx.NewDirectRestyClient(opts)
	}
	return &SubtitleCatProvider{origin: defaultSubtitleCatOrigin, client: client}
}

func (p *SubtitleCatProvider) Name() string {
	return "SubtitleCat"
}

type catSearchEntry struct {
	Name      string
	DetailURL string
}

func (p *SubtitleCatProvider) Search(ctx context.Context, code string) ([]Candidate, error) {
	clean := strings.TrimSpace(code)
	if clean == "" {
		return nil, nil
	}

	searchURL := fmt.Sprintf("%s/index.php?search=%s", p.origin, url.QueryEscape(clean))
	resp, err := p.client.R().SetContext(ctx).Get(searchURL)
	if err != nil {
		return nil, fmt.Errorf("subtitlecat search request: %w", err)
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("subtitlecat search status %d", resp.StatusCode())
	}

	entries := parseSubtitleCatSearch(resp.Body(), p.origin)
	if len(entries) == 0 {
		return nil, nil
	}

	if len(entries) > 5 {
		entries = entries[:5]
	}

	var candidates []Candidate
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return candidates, ctx.Err()
		default:
		}

		detailResp, err := p.client.R().SetContext(ctx).Get(entry.DetailURL)
		if err != nil || detailResp.StatusCode() != 200 {
			continue
		}

		items := parseSubtitleCatDetail(detailResp.Body(), entry.Name, p.origin)
		candidates = append(candidates, items...)
	}

	return candidates, nil
}

func parseSubtitleCatSearch(body []byte, origin string) []catSearchEntry {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}

	var entries []catSearchEntry
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			href := getAttr(n, "href")
			if strings.Contains(href, "subs/") {
				name := extractText(n)
				if name != "" {
					fullURL := href
					if !strings.HasPrefix(href, "http") {
						fullURL = origin + "/" + strings.TrimPrefix(href, "/")
					}
					entries = append(entries, catSearchEntry{
						Name:      name,
						DetailURL: fullURL,
					})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return entries
}

func parseSubtitleCatDetail(body []byte, movieName string, origin string) []Candidate {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}

	var candidates []Candidate
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && hasClass(n, "sub-single") {
			var flagAlt string
			var downloadURL string

			var innerWalk func(*html.Node)
			innerWalk = func(in *html.Node) {
				if in.Type == html.ElementNode {
					if in.Data == "img" && hasClass(in, "flag") {
						flagAlt = getAttr(in, "alt")
					}
					if in.Data == "a" && hasClass(in, "green-link") {
						href := getAttr(in, "href")
						if strings.HasSuffix(strings.ToLower(href), ".srt") {
							if !strings.HasPrefix(href, "http") {
								downloadURL = origin + "/" + strings.TrimPrefix(href, "/")
							} else {
								downloadURL = href
							}
						}
					}
				}
				for c := in.FirstChild; c != nil; c = c.NextSibling {
					innerWalk(c)
				}
			}
			innerWalk(n)

			if downloadURL != "" {
				lang := LangUnknown
				switch strings.ToLower(flagAlt) {
				case "zh-cn":
					lang = LangSimplifiedChinese
				case "zh-tw":
					lang = LangTraditionalChinese
				}

				if lang != LangUnknown {
					candidates = append(candidates, Candidate{
						Provider: "SubtitleCat",
						Name:     movieName,
						URL:      downloadURL,
						Format:   "srt",
						Language: lang,
						Version:  DetectVersion(movieName),
					})
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return candidates
}

func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, targetClass string) bool {
	classAttr := getAttr(n, "class")
	for _, c := range strings.Fields(classAttr) {
		if c == targetClass {
			return true
		}
	}
	return false
}

func extractText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(sb.String())
}
