package providers

import (
	"context"
	"encoding/json"
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
	movieID := strings.ReplaceAll(strings.TrimPrefix(code, "1PONDO-"), "-", "_")
	return parsePondoMovieJSON(body, code, movieID, "onepondo", 2470)
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
