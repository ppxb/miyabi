package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
)

// OnePondo adapts 1pondo's public movie JSON into the metadata provider contract.
type OnePondo struct {
	client client
}

func NewOnePondo(proxy *netx.ProxyManager) *OnePondo {
	return &OnePondo{client: newClient(proxy, "https://www.1pondo.tv/", "", "1pondo.tv")}
}

func (*OnePondo) ID() string { return "onepondo" }

var onePondoCode = regexp.MustCompile(`^\d{6}-\d{3}$`)

func (*OnePondo) Supports(code string) bool {
	upper := strings.ToUpper(code)
	return strings.HasPrefix(upper, "1PONDO-") || onePondoCode.MatchString(upper)
}

func (s *OnePondo) Media(ctx context.Context, target string) (domain.Media, error) {
	return s.client.Media(ctx, target)
}

func (s *OnePondo) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	if !s.Supports(code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	movieID := strings.ReplaceAll(strings.TrimPrefix(code, "1PONDO-"), "-", "_")
	body, err := s.client.get(ctx, "https://www.1pondo.tv/dyn/phpauto/movie_details/movie_id/"+movieID+".json")
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return parseOnePondoJSON(body, code)
}

func parseOnePondoJSON(body []byte, code string) (domain.MovieMetadata, error) {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return domain.MovieMetadata{}, fmt.Errorf("parse 1pondo response: %w", err)
	}
	title := firstText(doc, "Title", "TitleEn")
	cover := firstText(doc, "ThumbHigh", "ThumbUltra", "MovieThumb", "ThumbLow")
	movie := domain.Movie{
		Code:        code,
		Title:       title,
		ReleaseDate: releaseDate(firstText(doc, "Release")),
		// 1pondo reports duration in seconds; the domain stores minutes.
		Duration: intFromAny(firstValue(doc, "Duration")) / 60,
		Cover:    cover,
		Sources:  []domain.SourceID{{Provider: "onepondo", ID: code}},
	}
	movie.Thumbnail = movie.Cover
	for _, name := range stringValues(firstValue(doc, "ActressesJa", "ActressesEn")) {
		movie.Actors = append(movie.Actors, domain.Actor{Provider: "onepondo", ID: name, Name: name})
	}
	for _, name := range stringValues(firstValue(doc, "UCNAME", "UCNAMEEn")) {
		movie.Tags = append(movie.Tags, domain.Tag{Provider: "onepondo", ID: name, Name: name})
	}
	if series := firstText(doc, "Series", "SeriesEn"); series != "" {
		movie.Series = &domain.Series{Provider: "onepondo", ID: series, Name: series}
	}

	result := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: movie}}
	if movie.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "onepondo", URL: movie.Cover, Role: "cover"})
	}
	return result, nil
}

func firstValue(doc map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := doc[key]; ok {
			return value
		}
	}
	return nil
}

func firstText(doc map[string]any, keys ...string) string {
	return textFromAny(firstValue(doc, keys...))
}

func textFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.Itoa(int(typed))
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case string:
		return integer(typed)
	case json.Number:
		parsed, _ := strconv.Atoi(typed.String())
		return parsed
	default:
		return 0
	}
}

func stringValues(value any) []string {
	entries, ok := value.([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if name := textFromAny(entry); name != "" {
			names = append(names, name)
		}
	}
	return names
}
