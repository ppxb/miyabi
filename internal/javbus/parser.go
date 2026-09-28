package javbus

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
	"golang.org/x/net/html"
)

var (
	gidPattern = regexp.MustCompile(`var\s+gid\s*=\s*(\d+);`)
	ucPattern  = regexp.MustCompile(`var\s+uc\s*=\s*(\d+);`)
	imgPattern = regexp.MustCompile(`var\s+img\s*=\s*['"]([^'"]+)['"];`)
	hashRegex  = regexp.MustCompile(`(?i)xt=urn:btih:([a-f0-9]{40})`)
	sizeRegex  = regexp.MustCompile(`(?i)^([0-9]+(?:\.[0-9]+)?)\s*([kmgt]?b)$`)
	dateRegex  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

type detailParams struct {
	GID string
	UC  string
	Img string
}

func extractDetailParams(content string) (detailParams, error) {
	gidMatch := gidPattern.FindStringSubmatch(content)
	if len(gidMatch) < 2 {
		return detailParams{}, domain.E(domain.KindUpstream, "missing gid in JavBus detail page", nil)
	}

	ucMatch := ucPattern.FindStringSubmatch(content)
	if len(ucMatch) < 2 {
		return detailParams{}, domain.E(domain.KindUpstream, "missing uc in JavBus detail page", nil)
	}

	imgMatch := imgPattern.FindStringSubmatch(content)
	if len(imgMatch) < 2 {
		return detailParams{}, domain.E(domain.KindUpstream, "missing img in JavBus detail page", nil)
	}

	return detailParams{
		GID: gidMatch[1],
		UC:  ucMatch[1],
		Img: imgMatch[1],
	}, nil
}

func isNotFoundPage(content string) bool {
	return strings.Contains(content, "404 Page Not Found")
}

func isDriverVerify(content string) bool {
	return strings.Contains(content, "/doc/driver-verify") || strings.Contains(content, "driver-verify")
}

func isCloudflareChallenge(content string) bool {
	return strings.Contains(content, "Just a moment...") ||
		strings.Contains(content, "Attention Required! | Cloudflare") ||
		strings.Contains(content, "challenge-platform")
}

func parseMagnetsHTML(body string) ([]domain.Magnet, error) {
	// A bare sequence of <tr> elements must be enclosed within a <table>
	// for the HTML parser to construct the appropriate table context.
	doc, err := html.Parse(strings.NewReader("<table>" + body + "</table>"))
	if err != nil {
		return nil, fmt.Errorf("parse JavBus magnets html: %w", err)
	}

	var trs []*html.Node
	var collectTRs func(*html.Node)
	collectTRs = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "tr") {
			trs = append(trs, n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			collectTRs(c)
		}
	}
	collectTRs(doc)

	var magnets []domain.Magnet
	for _, tr := range trs {
		var tds []*html.Node
		for c := tr.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && strings.EqualFold(c.Data, "td") {
				tds = append(tds, c)
			}
		}
		if len(tds) < 3 {
			continue
		}

		magnetLink, name, hasSub, hd := parseFirstTD(tds[0])
		if magnetLink == "" {
			continue
		}
		hash := extractHash(magnetLink)
		if hash == "" {
			continue
		}

		sizeText := getElementText(tds[1])
		size := parseSize(sizeText)

		dateText := strings.TrimSpace(getElementText(tds[2]))
		createdAt := cleanDate(dateText)

		var tags []string
		if hasSub {
			tags = append(tags, domain.MagnetTagSubtitle)
		}
		if hd {
			tags = append(tags, domain.MagnetTagHD)
		}

		magnets = append(magnets, domain.Magnet{
			Hash:        hash,
			Name:        name,
			Size:        size,
			HasSubtitle: hasSub,
			HD:          hd,
			FilesCount:  0,
			CreatedAt:   createdAt,
			Sources:     []string{domain.MagnetSourceJavBus},
			Tags:        tags,
			Inferred:    false,
		})
	}

	return magnets, nil
}

func parseFirstTD(n *html.Node) (link, name string, hasSub, hd bool) {
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			if strings.EqualFold(node.Data, "a") {
				for _, attr := range node.Attr {
					if attr.Key == "href" && strings.HasPrefix(attr.Val, "magnet:?") {
						if link == "" {
							link = attr.Val
						}
					}
					if attr.Key == "class" {
						for _, cls := range strings.Fields(attr.Val) {
							if cls == "btn-primary" {
								hd = true
							}
							if cls == "btn-warning" {
								hasSub = true
							}
						}
					}
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	if link != "" {
		if u, err := url.Parse(link); err == nil {
			dn := u.Query().Get("dn")
			if dn != "" {
				name = dn
			}
		}
	}
	if name == "" {
		name = strings.TrimSpace(getElementText(n))
	}
	return link, name, hasSub, hd
}

func extractHash(link string) string {
	match := hashRegex.FindStringSubmatch(link)
	if len(match) > 1 {
		return strings.ToLower(match[1])
	}
	return ""
}

func parseSize(str string) int64 {
	str = strings.TrimSpace(str)
	match := sizeRegex.FindStringSubmatch(str)
	if len(match) != 3 {
		return 0
	}
	val, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0
	}
	unit := strings.ToUpper(match[2])
	multiplier := float64(1)
	switch unit {
	case "B":
		multiplier = 1
	case "KB":
		multiplier = 1024
	case "MB":
		multiplier = 1024 * 1024
	case "GB":
		multiplier = 1024 * 1024 * 1024
	case "TB":
		multiplier = 1024 * 1024 * 1024 * 1024
	}
	return int64(val * multiplier)
}

func cleanDate(str string) string {
	str = strings.TrimSpace(str)
	if str == "0000-00-00" {
		return ""
	}
	if dateRegex.MatchString(str) {
		return str
	}
	return ""
}

func getElementText(n *html.Node) string {
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
	return sb.String()
}
