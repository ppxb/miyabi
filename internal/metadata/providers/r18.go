package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
)

// R18 adapts r18.dev's public JSON endpoint into the metadata provider contract.
type R18 struct {
	client client
}

func NewR18(proxy *netx.ProxyManager) *R18 {
	return &R18{client: newClient(proxy, "https://r18.dev/", "", "r18.dev")}
}

func (*R18) ID() string { return "r18dev" }

func (*R18) Supports(code string) bool {
	upper := strings.ToUpper(code)
	return upper != "" && !strings.HasPrefix(upper, "FC2") && !strings.HasPrefix(upper, "HEYZO")
}

func (s *R18) Media(ctx context.Context, target string) (domain.Media, error) {
	return s.client.Media(ctx, target)
}

type r18Named struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type r18Response struct {
	ContentID      string `json:"content_id"`
	DVDID          string `json:"dvd_id"`
	Title          string `json:"title"`
	ReleaseDate    string `json:"release_date"`
	RuntimeMinutes string `json:"runtime_minutes"`
	Images         struct {
		CoverImage struct {
			Large string `json:"large"`
		} `json:"cover_image"`
	} `json:"images"`
	Gallery []struct {
		Large string `json:"large"`
	} `json:"gallery"`
	Actresses  []r18Named `json:"actresses"`
	Categories []r18Named `json:"categories"`
	Maker      *r18Named  `json:"maker"`
	Label      *r18Named  `json:"label"`
	Series     *r18Named  `json:"series"`
	Directors  []r18Named `json:"directors"`
}

func (s *R18) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	if code == "" || !s.Supports(code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	for _, contentID := range r18ContentIDs(code) {
		body, err := s.client.get(ctx, "https://r18.dev/videos/vod/movies/detail/-/id="+contentID+"/json")
		if errors.Is(err, metadata.ErrNotFound) {
			continue
		}
		if err != nil {
			return domain.MovieMetadata{}, err
		}
		return parseR18JSON(body, code, contentID)
	}
	return domain.MovieMetadata{}, metadata.ErrNotFound
}

func r18ContentIDs(code string) []string {
	compact := strings.ToLower(strings.ReplaceAll(code, "-", ""))
	split := 0
	for split < len(compact) && (compact[split] < '0' || compact[split] > '9') {
		split++
	}
	if split == 0 || split == len(compact) {
		return []string{compact}
	}
	label, number := compact[:split], compact[split:]
	padded := label + strings.Repeat("0", max(0, 5-len(number))) + number
	if padded == label+number {
		return []string{padded}
	}
	return []string{padded, label + number}
}

func parseR18JSON(body []byte, code, contentID string) (domain.MovieMetadata, error) {
	var doc r18Response
	if err := json.Unmarshal(body, &doc); err != nil {
		return domain.MovieMetadata{}, fmt.Errorf("parse r18.dev response: %w", err)
	}
	if doc.DVDID != "" && !codeid.IsFormatEquivalent(doc.DVDID, code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	movie := domain.Movie{
		Code:        code,
		Title:       doc.Title,
		ReleaseDate: releaseDate(doc.ReleaseDate),
		Duration:    integer(doc.RuntimeMinutes),
		Cover:       doc.Images.CoverImage.Large,
		Sources:     []domain.SourceID{{Provider: "r18dev", ID: contentID}},
	}
	movie.Thumbnail = movie.Cover
	for _, actor := range doc.Actresses {
		if actor.Name != "" {
			movie.Actors = append(movie.Actors, domain.Actor{Provider: "r18dev", ID: actor.ID, Name: actor.Name})
		}
	}
	for _, category := range doc.Categories {
		if category.Name != "" {
			movie.Tags = append(movie.Tags, domain.Tag{Provider: "r18dev", ID: category.ID, Name: category.Name})
		}
	}
	if doc.Maker != nil && doc.Maker.Name != "" {
		movie.Maker = &domain.Maker{Provider: "r18dev", ID: doc.Maker.ID, Name: doc.Maker.Name}
	} else if doc.Label != nil && doc.Label.Name != "" {
		movie.Maker = &domain.Maker{Provider: "r18dev", ID: doc.Label.ID, Name: doc.Label.Name}
	}
	if doc.Series != nil && doc.Series.Name != "" {
		movie.Series = &domain.Series{Provider: "r18dev", ID: doc.Series.ID, Name: doc.Series.Name}
	}
	if len(doc.Directors) > 0 && doc.Directors[0].Name != "" {
		movie.Director = &domain.Director{Provider: "r18dev", ID: doc.Directors[0].ID, Name: doc.Directors[0].Name}
	}

	result := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: movie}}
	if movie.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "r18dev", URL: movie.Cover, Role: "cover"})
	}
	for _, image := range doc.Gallery {
		if image.Large != "" {
			result.Images = append(result.Images, domain.ImageCandidate{Provider: "r18dev", URL: image.Large, Role: "preview"})
		}
	}
	return result, nil
}
