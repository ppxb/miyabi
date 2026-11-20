package actors

import (
	"regexp"
	"strconv"
	"strings"

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
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return result
}

func first(n *html.Node, match func(*html.Node) bool) *html.Node {
	var found *html.Node
	var walk func(*html.Node) bool
	walk = func(node *html.Node) bool {
		if node == nil {
			return false
		}
		if node.Type == html.ElementNode && match(node) {
			found = node
			return true
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			if walk(c) {
				return true
			}
		}
		return false
	}
	walk(n)
	return found
}

func tag(n *html.Node, name string) *html.Node {
	return first(n, func(n *html.Node) bool { return n.Data == name })
}

func elementByID(n *html.Node, id string) *html.Node {
	return first(n, func(n *html.Node) bool { return attr(n, "id") == id })
}

func text(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Data == "script" || n.Data == "style" {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

var numberRe = regexp.MustCompile(`\d+`)

func integer(s string) int {
	v, _ := strconv.Atoi(numberRe.FindString(s))
	return v
}
