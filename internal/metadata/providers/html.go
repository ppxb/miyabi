package providers

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

func attr(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func hasClass(n *html.Node, name string) bool {
	return strings.Contains(" "+attr(n, "class")+" ", " "+name+" ")
}
func nodes(n *html.Node, match func(*html.Node) bool) []*html.Node {
	var result []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode && match(n) {
			result = append(result, n)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return result
}
func first(n *html.Node, match func(*html.Node) bool) *html.Node {
	for _, found := range nodes(n, match) {
		return found
	}
	return nil
}
func tag(n *html.Node, name string) *html.Node {
	return first(n, func(n *html.Node) bool { return n.Data == name })
}
func class(n *html.Node, name string) *html.Node {
	return first(n, func(n *html.Node) bool { return hasClass(n, name) })
}
func id(n *html.Node, name string) *html.Node {
	return first(n, func(n *html.Node) bool { return attr(n, "id") == name })
}
func text(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		style := strings.ReplaceAll(attr(n, "style"), " ", "")
		if n.Data == "script" || n.Data == "style" || strings.Contains(style, "display:none") || strings.Contains(style, "zoom:0.01") {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}
func meta(n *html.Node, key string) string {
	return attr(first(n, func(n *html.Node) bool {
		return n.Data == "meta" && (attr(n, "property") == key || attr(n, "name") == key)
	}), "content")
}
func absolute(base, ref string) string {
	if ref == "" {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	return b.ResolveReference(u).String()
}

var number = regexp.MustCompile(`\d+(?:\.\d+)?`)
var date = regexp.MustCompile(`\d{4}[-/]\d{2}[-/]\d{2}`)

func integer(s string) int { v, _ := strconv.Atoi(number.FindString(s)); return v }
func releaseDate(s string) string {
	s = strings.ReplaceAll(date.FindString(s), "/", "-")
	if _, err := time.Parse(time.DateOnly, s); err != nil {
		return ""
	}
	return s
}
