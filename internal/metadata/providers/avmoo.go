package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
)

const (
	avmooLanguage = "ja"
	avmooOK       = 200
)

// Avmoo adapts avmoo.shop's private JSON API, a JavBus-class catalogue
// aggregator. Fetch resolves the catalogue number through the site's POST
// search, then reads the full movie document with cast, genres and images.
type Avmoo struct{ client }

func NewAvmoo(proxy *netx.ProxyManager) *Avmoo {
	return &Avmoo{newClient(proxy, "https://avmoo.shop/", "", "avmoo.shop", "netcdn.space")}
}

func (*Avmoo) ID() string { return "avmoo" }

// Supports accepts every catalogue number; official FC2/HEYZO sources still
// come first in merge order, so AVMOO only supplements gaps.
func (*Avmoo) Supports(code string) bool {
	return strings.TrimSpace(code) != ""
}

func (s *Avmoo) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	if code == "" || !s.Supports(code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	movieID, err := s.resolveMovieID(ctx, code)
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	body, err := s.postJSON(ctx, s.base+"jav/data/api/getMovie", []any{movieID, avmooLanguage})
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return domain.MovieMetadata{}, domain.E(domain.KindUpstream, "AVMOO 详情接口不可用", errors.New("avmoo getMovie returned HTTP 404"))
		}
		return domain.MovieMetadata{}, err
	}
	var envelope avmooDetailEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return domain.MovieMetadata{}, fmt.Errorf("decode avmoo movie: %w", err)
	}
	switch {
	case envelope.Code == http.StatusNotFound:
		return domain.MovieMetadata{}, metadata.ErrNotFound
	case envelope.Code != avmooOK || envelope.Data.MovieID == "":
		return domain.MovieMetadata{}, domain.E(domain.KindUpstream, "AVMOO 返回了不完整的影片资料",
			fmt.Errorf("avmoo getMovie code=%d message=%s", envelope.Code, envelope.Message))
	}
	if !codeid.IsEquivalent(envelope.Data.MovieFanHao, code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	result := avmooMetadata(code, envelope.Data)
	if result.Detail.Title == "" {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	return result, nil
}

// resolveMovieID posts the catalogue number to the site search and returns the
// exact equivalent entry. The search also answers case-folded and separator-free
// spellings, but never falls back to a merely similar number.
func (s *Avmoo) resolveMovieID(ctx context.Context, code string) (string, error) {
	body, err := s.postJSON(ctx, s.base+"jav/data/api/search",
		[]any{map[string]string{"search": code, "lang": avmooLanguage}, 60, 1})
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return "", domain.E(domain.KindUpstream, "AVMOO 搜索接口不可用", errors.New("avmoo search returned HTTP 404"))
		}
		return "", err
	}
	var envelope avmooSearchEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", fmt.Errorf("decode avmoo search: %w", err)
	}
	if envelope.Code != avmooOK {
		return "", domain.E(domain.KindUpstream, "AVMOO 搜索失败", fmt.Errorf("code=%d message=%s", envelope.Code, envelope.Message))
	}
	for _, item := range envelope.Data {
		if item.MovieID != "" && codeid.IsEquivalent(item.MovieFanHao, code) {
			return item.MovieID, nil
		}
	}
	return "", metadata.ErrNotFound
}

func avmooMetadata(code string, source avmooMovie) domain.MovieMetadata {
	title := avmooPick(source.TitleJA, source.Title, source.TitleEN, source.TitleCN, source.TitleTW)
	m := domain.Movie{
		Code: code, Title: title,
		ReleaseDate: strings.TrimSpace(source.ReleaseDate),
		Duration:    source.Length,
		Cover:       source.PosterLarge, Thumbnail: source.PosterSmall,
		Sources: []domain.SourceID{{Provider: "avmoo", ID: source.MovieID}},
	}
	if origin := strings.TrimSpace(source.TitleJA); origin != "" {
		m.OriginTitle = origin
	}
	if m.Cover == "" {
		m.Cover = m.Thumbnail
	}
	if m.Thumbnail == "" {
		m.Thumbnail = m.Cover
	}
	for _, star := range source.Stars {
		name := avmooPick(star.NameJA, star.Name, star.NameEN, star.NameCN, star.NameTW)
		if name == "" {
			continue
		}
		m.Actors = append(m.Actors, domain.Actor{
			Provider: "avmoo", ID: star.ID, Name: name,
			NameZHT: avmooPick(star.NameTW, star.NameCN), Avatar: star.AvatarURL,
		})
	}
	for _, genre := range source.Genres {
		name := avmooPick(genre.NameJA, genre.Name, genre.NameEN, genre.NameCN, genre.NameTW)
		if name == "" {
			continue
		}
		m.Tags = append(m.Tags, domain.Tag{
			Provider: "avmoo", ID: genre.ID, Name: name,
			NameZHT: avmooPick(genre.NameTW, genre.NameCN),
		})
	}
	if source.Studio != nil {
		if name := avmooPick(source.Studio.NameJA, source.Studio.Name, source.Studio.NameEN, source.Studio.NameCN, source.Studio.NameTW); name != "" {
			m.Maker = &domain.Maker{Provider: "avmoo", ID: source.Studio.ID, Name: name}
		}
	}
	if m.Maker == nil && source.Label != nil {
		if name := avmooPick(source.Label.NameJA, source.Label.Name, source.Label.NameEN, source.Label.NameCN, source.Label.NameTW); name != "" {
			m.Maker = &domain.Maker{Provider: "avmoo", ID: source.Label.ID, Name: name}
		}
	}
	if source.Series != nil {
		if name := avmooPick(source.Series.NameJA, source.Series.Name, source.Series.NameEN, source.Series.NameCN, source.Series.NameTW); name != "" {
			m.Series = &domain.Series{Provider: "avmoo", ID: source.Series.ID, Name: name}
		}
	}
	if source.Director != nil {
		if name := avmooPick(source.Director.NameJA, source.Director.Name, source.Director.NameEN, source.Director.NameCN, source.Director.NameTW); name != "" {
			m.Director = &domain.Director{Provider: "avmoo", ID: source.Director.ID, Name: name}
		}
	}
	for index, url := range source.SampleLarge {
		if url == "" {
			continue
		}
		thumbnail := url
		if index < len(source.SampleSmall) && source.SampleSmall[index] != "" {
			thumbnail = source.SampleSmall[index]
		}
		m.PreviewImages = append(m.PreviewImages, domain.PreviewImage{Original: url, Thumbnail: thumbnail})
	}
	m.HasPreview = len(m.PreviewImages) > 0

	result := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: m}}
	for _, url := range []string{source.PosterLarge, source.PosterSmall} {
		if url == "" || avmooHasImage(result.Images, url) {
			continue
		}
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "avmoo", URL: url, Role: "cover"})
	}
	for _, image := range m.PreviewImages {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "avmoo", URL: image.Original, Thumbnail: image.Thumbnail, Role: "preview"})
	}
	return result
}

func avmooPick(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func avmooHasImage(images []domain.ImageCandidate, url string) bool {
	for _, image := range images {
		if image.URL == url {
			return true
		}
	}
	return false
}

type avmooSearchEnvelope struct {
	Code    int          `json:"code"`
	Message string       `json:"message"`
	Data    []avmooMovie `json:"data"`
}

type avmooDetailEnvelope struct {
	Code    int        `json:"code"`
	Message string     `json:"message"`
	Data    avmooMovie `json:"data"`
}

type avmooMovie struct {
	MovieID     string         `json:"movieId"`
	MovieFanHao string         `json:"movieFanHao"`
	Title       string         `json:"title"`
	TitleJA     string         `json:"title_ja"`
	TitleEN     string         `json:"title_en"`
	TitleCN     string         `json:"title_cn"`
	TitleTW     string         `json:"title_tw"`
	ReleaseDate string         `json:"releaseDate"`
	Length      int            `json:"length"`
	PosterLarge string         `json:"posterLarge"`
	PosterSmall string         `json:"posterSmall"`
	SampleLarge []string       `json:"sampleLarge"`
	SampleSmall []string       `json:"sampleSmall"`
	Label       *avmooLabel    `json:"label"`
	Series      *avmooSeries   `json:"series"`
	Studio      *avmooStudio   `json:"studio"`
	Director    *avmooDirector `json:"director"`
	Stars       []avmooStar    `json:"star"`
	Genres      []avmooGenre   `json:"genre"`
}

type avmooLabel struct {
	ID     string `json:"labelId"`
	Name   string `json:"labelName"`
	NameJA string `json:"labelName_ja"`
	NameEN string `json:"labelName_en"`
	NameCN string `json:"labelName_cn"`
	NameTW string `json:"labelName_tw"`
}

type avmooSeries struct {
	ID     string `json:"seriesId"`
	Name   string `json:"seriesName"`
	NameJA string `json:"seriesName_ja"`
	NameEN string `json:"seriesName_en"`
	NameCN string `json:"seriesName_cn"`
	NameTW string `json:"seriesName_tw"`
}

type avmooStudio struct {
	ID     string `json:"studioId"`
	Name   string `json:"studioName"`
	NameJA string `json:"studioName_ja"`
	NameEN string `json:"studioName_en"`
	NameCN string `json:"studioName_cn"`
	NameTW string `json:"studioName_tw"`
}

type avmooDirector struct {
	ID     string `json:"directorId"`
	Name   string `json:"directorName"`
	NameJA string `json:"directorName_ja"`
	NameEN string `json:"directorName_en"`
	NameCN string `json:"directorName_cn"`
	NameTW string `json:"directorName_tw"`
}

type avmooStar struct {
	ID        string `json:"starId"`
	Name      string `json:"starName"`
	NameJA    string `json:"starName_ja"`
	NameEN    string `json:"starName_en"`
	NameCN    string `json:"starName_cn"`
	NameTW    string `json:"starName_tw"`
	AvatarURL string `json:"avatarUrl"`
}

type avmooGenre struct {
	ID     string `json:"genreId"`
	Name   string `json:"genreName"`
	NameJA string `json:"genreName_ja"`
	NameEN string `json:"genreName_en"`
	NameCN string `json:"genreName_cn"`
	NameTW string `json:"genreName_tw"`
	Type   int    `json:"type"`
}
