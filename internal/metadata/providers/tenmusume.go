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

// TenMusume adapts 10musume.com. It shares the 1pondo JSON movie API used by
// onepondo, so only the host and id shape differ.
type TenMusume struct{ client }

func NewTenMusume(proxy *netx.ProxyManager) *TenMusume {
	return &TenMusume{newClient(proxy, "https://www.10musume.com/", "", "10musume.com")}
}

func (*TenMusume) ID() string { return "10musume" }

var tenMusumeCode = regexp.MustCompile(`^\d{6}[-_]\d{2}$`)

func (*TenMusume) Supports(code string) bool { return tenMusumeCode.MatchString(code) }

func (s *TenMusume) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	if !tenMusumeCode.MatchString(code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	movieID := strings.ReplaceAll(code, "-", "_")
	body, err := s.get(ctx, s.base+"dyn/phpauto/movie_details/movie_id/"+movieID+".json")
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return parsePondoMovieJSON(body, code, movieID, "10musume", 2471)
}

// parsePondoMovieJSON reads the shared 1pondo-family movie JSON (used by
// 10musume and muramura; onepondo has its own slightly older variant).
func parsePondoMovieJSON(body []byte, code, movieID, providerID string, expectedSiteID int) (domain.MovieMetadata, error) {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return domain.MovieMetadata{}, fmt.Errorf("parse %s response: %w", providerID, err)
	}
	// Hard identity: the JSON must name this very work and site, so a stray or
	// mismatched response cannot attach another title to the requested code.
	if firstText(doc, "MovieID") != movieID || intFromAny(firstValue(doc, "SiteID")) != expectedSiteID {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	title := firstText(doc, "Title", "TitleEn")
	if title == "" {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	cover := firstText(doc, "ThumbHigh", "ThumbUltra", "MovieThumb", "ThumbLow")
	m := domain.Movie{
		Code: code, Title: title, OriginTitle: title,
		Plot:        firstText(doc, "Desc", "DescEn"),
		ReleaseDate: releaseDate(firstText(doc, "Release")),
		// The API reports duration in seconds; the domain stores minutes.
		Duration: intFromAny(firstValue(doc, "Duration")) / 60,
		Cover:    cover,
		Sources:  []domain.SourceID{{Provider: providerID, ID: code}},
	}
	m.Thumbnail = cover
	for _, name := range stringValues(firstValue(doc, "ActressesJa", "ActressesEn")) {
		m.Actors = append(m.Actors, domain.Actor{Provider: providerID, ID: name, Name: name})
	}
	for _, name := range stringValues(firstValue(doc, "UCNAME", "UCNAMEEn")) {
		m.Tags = append(m.Tags, domain.Tag{Provider: providerID, ID: name, Name: name})
	}
	if series := firstText(doc, "Series", "SeriesEn"); series != "" {
		m.Series = &domain.Series{Provider: providerID, ID: series, Name: series}
	}
	if rating, ok := ratingFromAny(firstValue(doc, "AvgRating")); ok {
		m.Rating, m.RatingMax, m.RatingSource = rating, 5, providerID
	}

	result := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: m, Zone: domain.ZoneUncensored}}
	if cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: providerID, URL: cover, Role: "cover", Layout: domain.CoverSingle})
	}
	return result, nil
}

func ratingFromAny(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		if typed > 0 && typed <= 5 {
			return typed, true
		}
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil && f > 0 && f <= 5 {
			return f, true
		}
	}
	return 0, false
}
