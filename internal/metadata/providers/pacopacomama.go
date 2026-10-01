package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
)

const pacopacomamaBase = "https://www.pacopacomama.com/"

type Pacopacomama struct{ client }

func NewPacopacomama(proxy *netx.ProxyManager) *Pacopacomama {
	return &Pacopacomama{newClient(proxy, pacopacomamaBase, "", "pacopacomama.com")}
}

func (*Pacopacomama) ID() string { return "pacopacomama" }

var (
	pacopacomamaCode      = regexp.MustCompile(`^[0-9]{6}[-_][0-9]{3}$`)
	pacopacomamaImageName = regexp.MustCompile(`^[A-Za-z0-9_-]+\.(?:jpg|jpeg|png|webp)$`)
)

func (*Pacopacomama) Supports(code string) bool { return pacopacomamaCode.MatchString(code) }

type pacopacomamaName struct {
	NameJa string
	NameEn string
}

func (n pacopacomamaName) name() string {
	if name := strings.TrimSpace(n.NameJa); name != "" {
		return name
	}
	return strings.TrimSpace(n.NameEn)
}

type pacopacomamaMovie struct {
	MovieID                         string
	SiteID                          int
	Title, TitleEn, Release         string
	Duration                        int
	AvgRating                       float64
	Status, Gallery, HasGallery     bool
	ThumbUltra, ThumbHigh, ThumbMed string
	ThumbLow, MovieThumb            string
	ActorID, UC                     []int
	ActressesList, UcNameList       map[string]pacopacomamaName
	SeriesID                        int
	Series, SeriesEn                string
}

func (s *Pacopacomama) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	if !s.Supports(ref.Code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	id := strings.ReplaceAll(ref.Code, "-", "_")
	body, err := s.get(ctx, s.base+"dyn/phpauto/movie_details/movie_id/"+id+".json")
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	var movie pacopacomamaMovie
	if err := json.Unmarshal(body, &movie); err != nil {
		return domain.MovieMetadata{}, fmt.Errorf("解析 Pacopacomama 影片资料: %w", err)
	}
	if movie.MovieID != id || movie.SiteID != 2469 {
		return domain.MovieMetadata{}, fmt.Errorf("Pacopacomama 影片身份与请求不一致")
	}
	if !movie.Status {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	result := pacopacomamaMetadata(movie)
	if result.Detail.Title == "" {
		return domain.MovieMetadata{}, fmt.Errorf("Pacopacomama 未返回影片标题")
	}
	if !movie.Gallery && !movie.HasGallery {
		return result, nil
	}
	path := "dyn/dla/json/movie_gallery/" + id + ".json"
	if !movie.Gallery {
		path = "dyn/phpauto/movie_galleries/movie_id/" + id + ".json"
	}
	body, err = s.get(ctx, s.base+path)
	if errors.Is(err, metadata.ErrNotFound) {
		return result, nil
	}
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	if err := addPacopacomamaGallery(&result, body, !movie.Gallery); err != nil {
		return domain.MovieMetadata{}, err
	}
	return result, nil
}

func pacopacomamaMetadata(source pacopacomamaMovie) domain.MovieMetadata {
	title := (pacopacomamaName{source.Title, source.TitleEn}).name()
	m := domain.MovieDetail{Movie: domain.Movie{
		Code: source.MovieID, Title: title, OriginTitle: title,
		Sources:     []domain.SourceID{{Provider: "pacopacomama", ID: source.MovieID}},
		ReleaseDate: releaseDate(source.Release), Duration: source.Duration / 60,
		Maker: &domain.Maker{Provider: "pacopacomama", ID: strconv.Itoa(source.SiteID), Name: "Pacopacomama"},
	}, Zone: domain.ZoneUncensored}
	if source.AvgRating > 0 && source.AvgRating <= 5 {
		m.Rating, m.RatingMax, m.RatingSource = source.AvgRating, 5, "pacopacomama"
	}
	actors, tags := make(map[string]bool), make(map[string]bool)
	for _, actorID := range source.ActorID {
		id := strconv.Itoa(actorID)
		if name := source.ActressesList[id].name(); name != "" && !actors[id] {
			m.Actors = append(m.Actors, domain.Actor{Provider: "pacopacomama", ID: id, Name: name})
			actors[id] = true
		}
	}
	for _, tagID := range source.UC {
		id := strconv.Itoa(tagID)
		if name := source.UcNameList[id].name(); name != "" && !tags[id] {
			m.Tags = append(m.Tags, domain.Tag{Provider: "pacopacomama", ID: id, Name: name})
			tags[id] = true
		}
	}
	if name := (pacopacomamaName{source.Series, source.SeriesEn}).name(); source.SeriesID > 0 && name != "" {
		m.Series = &domain.Series{Provider: "pacopacomama", ID: strconv.Itoa(source.SeriesID), Name: name}
	}
	result := domain.MovieMetadata{Detail: m}
	seen := make(map[string]bool)
	for _, ref := range []string{source.ThumbUltra, source.ThumbHigh, source.ThumbMed, source.ThumbLow, source.MovieThumb} {
		u := pacopacomamaCoverURL(ref, source.MovieID)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "pacopacomama", URL: u, Role: "cover", Layout: domain.CoverSingle})
		if result.Detail.Cover == "" {
			result.Detail.Cover, result.Detail.Thumbnail = u, u
		}
	}
	return result
}

func pacopacomamaCoverURL(ref, movieID string) string {
	if ref == "" {
		return ""
	}
	u, err := url.Parse(absolute(pacopacomamaBase, ref))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" ||
		(u.Hostname() != "pacopacomama.com" && !strings.HasSuffix(u.Hostname(), ".pacopacomama.com")) {
		return ""
	}
	for _, prefix := range []string{"/moviepages/" + movieID + "/images/", "/assets/sample/" + movieID + "/"} {
		if name, ok := strings.CutPrefix(u.Path, prefix); ok && pacopacomamaImageName.MatchString(name) {
			return u.String()
		}
	}
	return ""
}

func addPacopacomamaGallery(result *domain.MovieMetadata, body []byte, filenames bool) error {
	var gallery struct {
		MovieID string
		Rows    []struct {
			Img, Filename string
			Protected     *bool
		}
	}
	if err := json.Unmarshal(body, &gallery); err != nil {
		return fmt.Errorf("解析 Pacopacomama 预览图: %w", err)
	}
	if gallery.MovieID != result.Detail.Code {
		return fmt.Errorf("Pacopacomama 预览图身份与影片不一致")
	}
	seen := make(map[string]bool)
	for _, row := range gallery.Rows {
		if row.Protected == nil || *row.Protected {
			continue
		}
		name, ok := strings.CutPrefix(row.Img, "movie_gallery/sample/"+gallery.MovieID+"/")
		prefix := pacopacomamaBase + "dyn/dla/images/movie_gallery/sample/" + gallery.MovieID + "/"
		if filenames {
			name, ok = row.Filename, true
			prefix = pacopacomamaBase + "assets/sample/" + gallery.MovieID + "/l/"
		}
		if !ok || !pacopacomamaImageName.MatchString(name) || seen[name] {
			continue
		}
		seen[name] = true
		u := prefix + name
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "pacopacomama", URL: u, Role: "preview"})
		result.Detail.PreviewImages = append(result.Detail.PreviewImages, domain.PreviewImage{Original: u, Thumbnail: u})
	}
	result.Detail.HasPreview = len(result.Detail.PreviewImages) > 0
	return nil
}
