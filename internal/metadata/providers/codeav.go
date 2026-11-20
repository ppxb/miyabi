package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

const codeAVBase = "https://www.codeav.net/"

// CodeAV adapts CodeAV's movie pages (JSON-LD) into the metadata provider contract.
type CodeAV struct {
	client client
}

func NewCodeAV(proxy *netx.ProxyManager) *CodeAV {
	return &CodeAV{client: newClient(proxy, codeAVBase, "", "codeav.net", "dmm.co.jp")}
}

func (*CodeAV) ID() string { return "codeav" }

func (*CodeAV) Supports(code string) bool { return strings.TrimSpace(code) != "" }

func (s *CodeAV) Media(ctx context.Context, target string) (domain.Media, error) {
	return s.client.Media(ctx, target)
}

func (s *CodeAV) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	if code == "" {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	for _, slug := range codeAVSlugs(code) {
		body, err := s.client.get(ctx, codeAVBase+"en/movie/"+slug)
		if err != nil {
			if err == metadata.ErrNotFound {
				continue
			}
			return domain.MovieMetadata{}, err
		}
		return parseCodeAVHTML(body, code)
	}
	return domain.MovieMetadata{}, metadata.ErrNotFound
}

func codeAVSlugs(code string) []string {
	lower := strings.ToLower(code)
	slugs := []string{lower}
	if base := strings.TrimSuffix(strings.ToUpper(lower), "CH"); base != strings.ToUpper(lower) {
		slugs = append(slugs, strings.ToLower(base))
	}
	return slugs
}

type codeAVPerson struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type codeAVMovie struct {
	Type          string `json:"@type"`
	Name          string `json:"name"`
	URL           string `json:"url"`
	DatePublished string `json:"datePublished"`
	Duration      string `json:"duration"`
	Description   string `json:"description"`
	Image         []struct {
		URL     string `json:"url"`
		Caption string `json:"caption"`
	} `json:"image"`
	Actor             json.RawMessage `json:"actor"`
	Director          json.RawMessage `json:"director"`
	ProductionCompany json.RawMessage `json:"productionCompany"`
	Genre             json.RawMessage `json:"genre"`
	Identifier        json.RawMessage `json:"identifier"`
	AggregateRating   struct {
		RatingValue json.RawMessage `json:"ratingValue"`
		BestRating  json.RawMessage `json:"bestRating"`
	} `json:"aggregateRating"`
}

func parseCodeAVHTML(body []byte, code string) (domain.MovieMetadata, error) {
	root, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	for _, script := range nodes(root, func(n *html.Node) bool {
		return n.Data == "script" && attr(n, "type") == "application/ld+json"
	}) {
		var doc codeAVMovie
		if err := json.Unmarshal([]byte(codeAVScriptText(script)), &doc); err != nil || doc.Type != "Movie" {
			continue
		}
		if product := codeAVProductCode(doc.Identifier); product != "" && !codeid.IsFormatEquivalent(product, code) {
			return domain.MovieMetadata{}, metadata.ErrNotFound
		}
		original := codeAVStripCode(doc.Name, code)
		if len(doc.Image) > 0 {
			if caption := codeAVStripCode(doc.Image[0].Caption, code); caption != "" {
				original = caption
			}
		}
		english := codeAVStripCode(doc.Name, code)
		movie := domain.Movie{
			Code:        code,
			Title:       original,
			OriginTitle: english,
			Plot:        strings.TrimSpace(doc.Description),
			ReleaseDate: releaseDate(doc.DatePublished),
			Duration:    codeAVDurationMinutes(doc.Duration),
			Sources:     []domain.SourceID{{Provider: "codeav", ID: code}},
		}
		if movie.Title == "" {
			movie.Title = english
		}
		if len(doc.Image) > 0 {
			movie.Cover = doc.Image[0].URL
			movie.Thumbnail = movie.Cover
		}
		for _, person := range codeAVPeople(doc.Actor) {
			if person.Name != "" {
				movie.Actors = append(movie.Actors, domain.Actor{Provider: "codeav", ID: codeAVLastSegment(person.URL), Name: person.Name})
			}
		}
		for _, name := range codeAVStrings(doc.Genre) {
			if name != "" {
				movie.Tags = append(movie.Tags, domain.Tag{Provider: "codeav", ID: name, Name: name})
			}
		}
		if people := codeAVPeople(doc.ProductionCompany); len(people) > 0 && people[0].Name != "" {
			movie.Maker = &domain.Maker{Provider: "codeav", ID: codeAVLastSegment(people[0].URL), Name: people[0].Name}
		}
		if people := codeAVPeople(doc.Director); len(people) > 0 && people[0].Name != "" {
			movie.Director = &domain.Director{Provider: "codeav", ID: codeAVLastSegment(people[0].URL), Name: people[0].Name}
		}
		if rating := codeAVFloat(doc.AggregateRating.RatingValue); rating > 0 {
			movie.Rating = rating
			movie.RatingMax = codeAVFloat(doc.AggregateRating.BestRating)
			if movie.RatingMax == 0 {
				movie.RatingMax = 5
			}
			movie.RatingSource = "codeav"
		}

		result := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: movie}}
		for index, image := range doc.Image {
			if image.URL == "" {
				continue
			}
			role := "preview"
			if index == 0 {
				role = "cover"
			}
			result.Images = append(result.Images, domain.ImageCandidate{Provider: "codeav", URL: image.URL, Role: role})
		}
		return result, nil
	}
	return domain.MovieMetadata{}, metadata.ErrNotFound
}

func codeAVScriptText(script *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			builder.WriteString(n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(script)
	return builder.String()
}

func codeAVStripCode(value, code string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{code + " – ", code + " - ", code + ": ", code + " "} {
		if strings.HasPrefix(strings.ToUpper(value), strings.ToUpper(prefix)) {
			return strings.TrimSpace(value[len(prefix):])
		}
	}
	return value
}

func codeAVPeople(raw json.RawMessage) []codeAVPerson {
	if len(raw) == 0 {
		return nil
	}
	var many []codeAVPerson
	if err := json.Unmarshal(raw, &many); err == nil {
		return many
	}
	var one codeAVPerson
	if err := json.Unmarshal(raw, &one); err == nil && one.Name != "" {
		return []codeAVPerson{one}
	}
	return nil
}

func codeAVStrings(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		return many
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil && one != "" {
		return []string{one}
	}
	return nil
}

func codeAVProductCode(raw json.RawMessage) string {
	var entries []struct {
		PropertyID string `json:"propertyID"`
		Value      string `json:"value"`
	}
	if err := json.Unmarshal(raw, &entries); err == nil {
		for _, entry := range entries {
			if entry.PropertyID == "product_code" {
				return entry.Value
			}
		}
	}
	var entry struct {
		PropertyID string `json:"propertyID"`
		Value      string `json:"value"`
	}
	if err := json.Unmarshal(raw, &entry); err == nil && entry.PropertyID == "product_code" {
		return entry.Value
	}
	return ""
}

func codeAVLastSegment(url string) string {
	url = strings.TrimSuffix(strings.TrimSpace(url), "/")
	if index := strings.LastIndex(url, "/"); index >= 0 {
		return url[index+1:]
	}
	return url
}

func codeAVFloat(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 0
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		return number
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(text), 64)
		return parsed
	}
	return 0
}

var codeAVDuration = regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?`)

func codeAVDurationMinutes(value string) int {
	match := codeAVDuration.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return 0
	}
	hours := integer(match[1])
	minutes := integer(match[2])
	return hours*60 + minutes
}
