package providers

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

// h0930Source is the shared implementation for the C0930 / H0930 / H4610 family:
// same EUC-JP moviepages layout, different host, id prefix and maker.
type h0930Source struct {
	client
	id           string
	siteHost     string
	prefix       string
	defaultMaker string
	gate         *regexp.Regexp
}

func NewC0930(proxy *netx.ProxyManager) *h0930Source {
	return &h0930Source{
		client: newClient(proxy, "https://www.c0930.com/", "", "c0930.com"), id: "c0930",
		siteHost: "c0930.com", prefix: "c0930", defaultMaker: "人妻斬り",
		gate: regexp.MustCompile(`^(?:ki\d{6}|hitozuma\d+)$`),
	}
}

func NewH0930(proxy *netx.ProxyManager) *h0930Source {
	return &h0930Source{
		client: newClient(proxy, "https://www.h0930.com/", "", "h0930.com"), id: "h0930",
		siteHost: "h0930.com", prefix: "h0930", defaultMaker: "H0930",
		gate: regexp.MustCompile(`^ori[a-z]*\d+$`),
	}
}

func NewH4610(proxy *netx.ProxyManager) *h0930Source {
	return &h0930Source{
		client: newClient(proxy, "https://www.h4610.com/", "", "h4610.com"), id: "h4610",
		siteHost: "h4610.com", prefix: "h4610", defaultMaker: "エッチな4610",
		gate: regexp.MustCompile(`^(?:tk|pla)\d{4}$`),
	}
}

func (s *h0930Source) ID() string { return s.id }

func (s *h0930Source) Supports(code string) bool {
	sid := h0930SiteID(code, s.prefix)
	return sid != "" && s.gate.MatchString(sid)
}

func (s *h0930Source) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	sid := h0930SiteID(code, s.prefix)
	if sid == "" || !s.gate.MatchString(sid) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	body, err := s.get(ctx, s.base+"moviepages/"+url.PathEscape(sid)+"/index.html")
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return parseH0930(decodeHTML(body), s.base, code, sid, s.siteHost, s.id, s.defaultMaker)
}

// h0930SiteID normalizes a catalogue code into the site's lowercase id: strip
// codeid-inserted separators and any site prefix (e.g. "C0930-KI220913").
func h0930SiteID(code, prefix string) string {
	s := strings.ToLower(strings.TrimSpace(code))
	s = strings.NewReplacer("-", "", "_", "").Replace(s)
	return strings.TrimPrefix(s, prefix)
}

var h0930GalleryHref = regexp.MustCompile(`href="(.+?)"`)

func parseH0930(decoded, base, code, siteID, siteHost, providerID, defaultMaker string) (domain.MovieMetadata, error) {
	doc, err := html.Parse(strings.NewReader(decoded))
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	play := elementByID(doc, "moviePlay")
	info := elementByID(doc, "movieInfo")
	// Hard identity: the player/info containers must exist and the page must
	// reference the requested id somewhere. A soft 404 / consent page lacks both.
	if (play == nil && info == nil) || !strings.Contains(decoded, siteID) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}

	title, plot, cover, release, duration, maker, rating := "", "", "", "", 0, defaultMaker, 0.0
	var actors []string

	for _, script := range nodes(doc, func(n *html.Node) bool {
		return n.Data == "script" && attr(n, "type") == "application/ld+json"
	}) {
		var data struct {
			Type        string `json:"@type"`
			Name        string `json:"name"`
			Image       string `json:"image"`
			Description string `json:"description"`
			Actor       struct {
				Name string `json:"name"`
			} `json:"actor"`
			Video struct {
				Duration string `json:"duration"`
				Actor    string `json:"actor"`
				Provider string `json:"provider"`
			} `json:"video"`
			ReleasedEvent struct {
				StartDate string `json:"startDate"`
			} `json:"releasedEvent"`
			AggregateRating struct {
				RatingValue string `json:"ratingValue"`
			} `json:"aggregateRating"`
		}
		if json.Unmarshal([]byte(strings.ReplaceAll(scriptText(script), "\n", "")), &data) != nil {
			continue
		}
		// Only consume the movie/video block; ignore WebSite/BreadcrumbList etc.
		if data.Type != "VideoObject" && data.Type != "Movie" && data.Video.Duration == "" && data.Description == "" {
			continue
		}
		title, plot = data.Name, data.Description
		if data.Image != "" {
			cover = data.Image
		}
		release = releaseDate(data.ReleasedEvent.StartDate)
		duration = japaneseDuration(data.Video.Duration)
		if data.Video.Provider != "" {
			maker = data.Video.Provider
		}
		for _, a := range []string{data.Actor.Name, data.Video.Actor} {
			if a = strings.TrimSpace(a); a != "" {
				actors = append(actors, splitNames(a)...)
			}
		}
		if v, e := strconv.ParseFloat(strings.TrimSpace(data.AggregateRating.RatingValue), 64); e == nil && v > 0 && v <= 5 {
			rating = v
		}
		break
	}

	// Title override from the visible heading when present.
	if play != nil {
		if h1 := tag(play, "h1"); h1 != nil {
			if span := tag(h1, "span"); span != nil {
				if t := strings.TrimSpace(text(span)); t != "" {
					title = t
				}
			} else if t := strings.TrimSpace(text(h1)); t != "" {
				title = t
			}
		}
	}
	if title == "" {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}

	m := domain.Movie{Code: code, Title: title, OriginTitle: title, Plot: plot, ReleaseDate: release, Duration: duration}
	if maker != "" {
		m.Maker = &domain.Maker{Provider: providerID, ID: maker, Name: maker}
	}
	seenActor := make(map[string]bool)
	for _, name := range actors {
		if name != "" && !seenActor[name] {
			seenActor[name] = true
			m.Actors = append(m.Actors, domain.Actor{Provider: providerID, ID: name, Name: name})
		}
	}
	if rating > 0 {
		m.Rating, m.RatingMax, m.RatingSource = rating, 5, providerID
	}

	if info != nil {
		seenTag := make(map[string]bool)
		for _, dl := range nodes(info, func(n *html.Node) bool { return n.Data == "dl" }) {
			for _, dt := range nodes(dl, func(n *html.Node) bool { return n.Data == "dt" }) {
				dd := nextSiblingTag(dt, "dd")
				if dd == nil {
					continue
				}
				switch strings.TrimRight(text(dt), "：: ") {
				case "公開日":
					if m.ReleaseDate == "" {
						m.ReleaseDate = releaseDate(text(dd))
					}
				case "動画":
					if m.Duration == 0 {
						m.Duration = japaneseDuration(text(dd))
					}
				case "プレイ内容":
					for _, name := range splitGenres(text(dd)) {
						if !seenTag[name] {
							seenTag[name] = true
							m.Tags = append(m.Tags, domain.Tag{Provider: providerID, ID: name, Name: name})
						}
					}
				}
			}
		}
	}

	if content := elementByID(doc, "movieContent"); content != nil {
		if poster := attr(content, "poster"); poster != "" {
			if u := h0930Image(base, poster, siteHost); u != "" {
				cover = u
			}
		}
	}
	if u := h0930Image(base, cover, siteHost); u != "" {
		m.Cover, m.Thumbnail = u, u
	}

	m.Sources = []domain.SourceID{{Provider: providerID, ID: code}}
	result := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: m, Zone: domain.ZoneUncensored}}
	if m.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: providerID, URL: m.Cover, Role: "cover", Layout: domain.CoverSingle})
	}
	if gallery := elementByID(doc, "movieGallery"); gallery != nil {
		seen := make(map[string]bool)
		for _, script := range nodes(gallery, func(n *html.Node) bool { return n.Data == "script" }) {
			for _, m2 := range h0930GalleryHref.FindAllStringSubmatch(scriptText(script), -1) {
				if strings.Contains(m2[1], "member") {
					continue
				}
				u := h0930Image(base, m2[1], siteHost)
				if u == "" || seen[u] {
					continue
				}
				seen[u] = true
				result.Images = append(result.Images, domain.ImageCandidate{Provider: providerID, URL: u, Role: "preview"})
				m.PreviewImages = append(m.PreviewImages, domain.PreviewImage{Original: u, Thumbnail: u})
			}
		}
	}
	result.Detail.PreviewImages = m.PreviewImages
	result.Detail.HasPreview = len(m.PreviewImages) > 0
	return result, nil
}

func splitGenres(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == '\u00a0' || r == '\u3000' || r == '、' || r == ',' || r == '/' || r == ' '
	})
	var out []string
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// scriptText returns the raw text of a <script>, which the shared text() helper
// intentionally skips. Used to read JSON-LD and inline gallery markup.
func scriptText(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	}
	return b.String()
}

func h0930Image(base, ref, siteHost string) string {
	if ref == "" {
		return ""
	}
	u, err := url.Parse(absolute(base, ref))
	if err != nil || u.Scheme != "https" || u.User != nil ||
		(u.Hostname() != siteHost && !strings.HasSuffix(u.Hostname(), "."+siteHost)) {
		return ""
	}
	return u.String()
}
