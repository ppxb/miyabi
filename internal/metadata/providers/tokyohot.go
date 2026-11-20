package providers

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

// TokyoHot adapts my.tokyo-hot.com product pages into the metadata provider
// contract. Tokyo-Hot uses short ids (n1633, kb1624, s2mbd-002, 5656) and the
// product page is plain UTF-8 HTML.
type TokyoHot struct{ client }

func NewTokyoHot(proxy *netx.ProxyManager) *TokyoHot {
	return &TokyoHot{newClient(proxy, "https://my.tokyo-hot.com/", "", "tokyo-hot.com")}
}

func (*TokyoHot) ID() string { return "tokyohot" }

// Tokyo-Hot's own short id families. Kept narrow so this single-studio source is
// not queried for unrelated catalogue numbers. Accepts the dash that codeid may
// insert between a short letter prefix and its digits (N-1633) for n/k/kb ids.
var (
	tokyoHotShort = regexp.MustCompile(`(?i)^(n|k|kb)-?(\d{3,4})$`)
	tokyoHotS2    = regexp.MustCompile(`(?i)^s2mbd-?(\d+)$`)
	tokyoHotNums  = regexp.MustCompile(`^\d{4}$`)
)

func tokyoHotID(code string) string {
	up := strings.ToUpper(strings.TrimSpace(code))
	if m := tokyoHotShort.FindStringSubmatch(up); m != nil {
		return strings.ToLower(m[1] + m[2])
	}
	if m := tokyoHotS2.FindStringSubmatch(up); m != nil {
		return "s2mbd-" + m[1]
	}
	if tokyoHotNums.MatchString(up) {
		return up
	}
	return ""
}

func (*TokyoHot) Supports(code string) bool { return tokyoHotID(code) != "" }

func (s *TokyoHot) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	id := tokyoHotID(code)
	if id == "" {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	body, err := s.get(ctx, s.base+"product/"+url.PathEscape(id)+"/?lang=ja")
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return parseTokyoHot(string(body), s.base, code)
}

func parseTokyoHot(body, base, code string) (domain.MovieMetadata, error) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	wantID := tokyoHotID(code)
	main := elementByID(doc, "main")
	if main == nil {
		main = doc
	}
	title := ""
	if contents := class(main, "contents"); contents != nil {
		if h := tag(contents, "h2"); h != nil {
			title = strings.TrimSpace(text(h))
		}
	}
	if title == "" {
		if h := tag(main, "h2"); h != nil {
			title = strings.TrimSpace(text(h))
		}
	}
	if title == "" {
		return domain.MovieMetadata{}, fmt.Errorf("TOKYO-HOT 详情页未返回标题")
	}
	info := class(main, "infowrapper")
	if info == nil {
		// A 200 page without the detail block is a soft not-found, not a movie.
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}

	m := domain.Movie{
		Code: code, Title: title, OriginTitle: title,
		Maker:   &domain.Maker{Provider: "tokyohot", ID: "tokyo-hot", Name: "TOKYO-HOT"},
		Sources: []domain.SourceID{{Provider: "tokyohot", ID: code}},
	}
	seenActor, seenTag := make(map[string]bool), make(map[string]bool)
	number := ""
	for _, dl := range nodes(info, func(n *html.Node) bool { return n.Data == "dl" }) {
		for _, dt := range nodes(dl, func(n *html.Node) bool { return n.Data == "dt" }) {
			dd := nextSiblingTag(dt, "dd")
			if dd == nil {
				continue
			}
			label, value := strings.TrimRight(text(dt), "：: "), text(dd)
			switch label {
			case "出演者":
				for _, name := range fieldNames(dd) {
					if !seenActor[name] {
						seenActor[name] = true
						m.Actors = append(m.Actors, domain.Actor{Provider: "tokyohot", ID: name, Name: name})
					}
				}
			case "プレイ内容", "タグ":
				for _, name := range fieldNames(dd) {
					if !seenTag[name] {
						seenTag[name] = true
						m.Tags = append(m.Tags, domain.Tag{Provider: "tokyohot", ID: name, Name: name})
					}
				}
			case "シリーズ":
				if value != "" {
					m.Series = &domain.Series{Provider: "tokyohot", ID: value, Name: value}
				}
			case "配信開始日":
				m.ReleaseDate = releaseDate(value)
			case "収録時間":
				m.Duration = japaneseDuration(value)
			case "作品番号":
				number = value
			}
		}
	}
	// The page's own 作品番号 must agree with the id we requested, so a mapping
	// mistake cannot attach another title's metadata to this code.
	if wantID == "" || number == "" || tokyoHotID(number) != wantID {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}

	cover, thumb, previews := tokyoHotMedia(base, doc, wantID)
	m.Cover, m.Thumbnail = cover, thumb
	if m.Cover == "" {
		m.Cover = m.Thumbnail
	}
	if m.Thumbnail == "" {
		m.Thumbnail = m.Cover
	}

	result := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: m, Zone: domain.ZoneUncensored}}
	if m.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "tokyohot", URL: m.Cover, Role: "cover", Layout: domain.CoverSingle})
	}
	for _, p := range previews {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "tokyohot", URL: p, Role: "preview"})
		m.PreviewImages = append(m.PreviewImages, domain.PreviewImage{Original: p, Thumbnail: p})
	}
	result.Detail.PreviewImages = m.PreviewImages
	result.Detail.HasPreview = len(m.PreviewImages) > 0
	return result, nil
}

// tokyoHotMedia picks the jacket image (cover), the package thumb and every
// rel="cap" preview. Only URLs that belong to this work's id (…/<id>/…) and look
// like image files are accepted, so related titles or banners cannot be picked.
func tokyoHotMedia(base string, root *html.Node, id string) (cover, thumb string, previews []string) {
	folder := "/" + strings.ToLower(id) + "/"
	pick := func(raw string) string {
		u, err := url.Parse(absolute(base, raw))
		if err != nil || u.Scheme != "https" || u.User != nil ||
			(u.Hostname() != "tokyo-hot.com" && !strings.HasSuffix(u.Hostname(), ".tokyo-hot.com")) {
			return ""
		}
		if !strings.Contains(strings.ToLower(u.Path), folder) {
			return ""
		}
		return u.String()
	}
	classify := func(u string) {
		if u == "" || !isImagePath(u) {
			return
		}
		if strings.Contains(u, "/jacket/") && cover == "" {
			cover = u
		}
		if strings.Contains(u, "/package/") && thumb == "" && (strings.HasSuffix(u, "_v.jpg") || strings.HasSuffix(u, "_vb.jpg")) {
			thumb = u
		}
	}
	for _, a := range nodes(root, func(n *html.Node) bool { return n.Data == "a" }) {
		classify(pick(attr(a, "href")))
	}
	for _, img := range nodes(root, func(n *html.Node) bool { return n.Data == "img" }) {
		classify(pick(attr(img, "src")))
	}
	seen := make(map[string]bool)
	for _, a := range nodes(root, func(n *html.Node) bool { return n.Data == "a" && attr(n, "rel") == "cap" }) {
		if u := pick(attr(a, "href")); u != "" && isImagePath(u) && !seen[u] {
			seen[u] = true
			previews = append(previews, u)
		}
	}
	return cover, thumb, previews
}

func isImagePath(u string) bool {
	p := strings.ToLower(u)
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	return strings.HasSuffix(p, ".jpg") || strings.HasSuffix(p, ".jpeg") || strings.HasSuffix(p, ".png") || strings.HasSuffix(p, ".webp")
}

// elementByID returns the first element carrying the given id, or nil.
func elementByID(root *html.Node, id string) *html.Node {
	return first(root, func(n *html.Node) bool { return attr(n, "id") == id })
}

// nextSiblingTag returns the next dd element sibling of a dt, or nil.
func nextSiblingTag(n *html.Node, name string) *html.Node {
	for s := n.NextSibling; s != nil; s = s.NextSibling {
		if s.Type == html.ElementNode && s.Data == name {
			return s
		}
	}
	return nil
}

// fieldNames reads a definition value: linked names if present, otherwise the
// text split on Japanese/ASCII list separators.
func fieldNames(dd *html.Node) []string {
	if links := nodes(dd, func(n *html.Node) bool { return n.Data == "a" }); len(links) > 0 {
		var names []string
		for _, a := range links {
			if name := strings.TrimSpace(text(a)); name != "" {
				names = append(names, name)
			}
		}
		return names
	}
	return splitNames(text(dd))
}

func splitNames(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == '、' || r == ',' || r == '/' || r == ' ' || r == '\u3000'
	})
	var names []string
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			names = append(names, f)
		}
	}
	return names
}

// japaneseDuration turns "01:38:38" / "1時間20分" / "120分" / "PT1H20M" into
// whole minutes.
func japaneseDuration(v string) int {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(strings.ToUpper(v), "PT") {
		return codeAVDurationMinutes(v)
	}
	if strings.Contains(v, ":") {
		total := 0
		for _, p := range strings.Split(v, ":") {
			total = total*60 + integer(p)
		}
		return total / 60
	}
	if i := strings.Index(v, "時間"); i >= 0 {
		return integer(v[:i])*60 + integer(v[i+len("時間"):])
	}
	return integer(v)
}
