package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
)

type FANZA struct {
	client
	endpoint string
}

func NewFANZA(proxy *netx.ProxyManager) *FANZA {
	return &FANZA{
		client:   newClient(proxy, "https://video.dmm.co.jp/", "", "dmm.co.jp"),
		endpoint: "https://api.video.dmm.co.jp/graphql",
	}
}

func (*FANZA) ID() string { return "fanza" }

var fanzaNumber = regexp.MustCompile(`^([0-9]*[A-Z][A-Z0-9]*)-([0-9]+)([A-Z]?)$`)

func (*FANZA) Supports(code string) bool {
	return fanzaNumber.MatchString(code) && !strings.HasPrefix(code, "FC2-")
}

const fanzaSearchQuery = `query MovieSearch($keyword: String!, $offset: Int!) {
  legacySearchPPV(limit: 40, offset: $offset, sort: RECOMMENDED,
    queryWord: $keyword, includeExplicit: true, excludeUndelivered: false) {
    result { contents { id } pageInfo { hasNext } }
  }
}`

const fanzaDetailQuery = `query MovieDetail($id: ID!) {
  ppvContent(id: $id) {
    id makerContentId floor title duration deliveryStartDate makerReleasedAt
    packageImage { largeUrl mediumUrl }
    sampleImages { imageUrl largeImageUrl }
    actresses { id name imageUrl }
    histrions { id name }
    amateurActress { id name imageUrl }
    maker { id name } series { id name } directors { id name } genres { id name }
  }
  reviewSummary(contentId: $id) { average }
}`

type fanzaIdentity struct {
	ID   string `json:"id"`
	Code string `json:"makerContentId"`
}

type fanzaEntity struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl"`
}

type fanzaMovie struct {
	fanzaIdentity
	Floor, Title                       string
	Duration                           int
	DeliveryStartDate, MakerReleasedAt *time.Time
	PackageImage                       struct {
		LargeURL  string `json:"largeUrl"`
		MediumURL string `json:"mediumUrl"`
	}
	SampleImages []struct {
		ImageURL      string `json:"imageUrl"`
		LargeImageURL string `json:"largeImageUrl"`
	}
	Actresses, Histrions []fanzaEntity
	AmateurActress       *fanzaEntity
	Maker, Series        *fanzaEntity
	Directors, Genres    []fanzaEntity
}

func (s *FANZA) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	parts := fanzaNumber.FindStringSubmatch(code)
	if parts == nil || !s.Supports(code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	// These are search terms, never assumed content IDs. The API's official
	// makerContentId must still confirm every candidate, including distributor IDs.
	digital := strings.ToLower(parts[1] + strings.Repeat("0", max(0, 5-len(parts[2]))) + parts[2] + parts[3])
	compact := strings.ToLower(parts[1] + parts[2] + parts[3])
	queries := []string{digital}
	if compact != digital {
		queries = append(queries, compact)
	}
	for _, keyword := range queries {
		identities, err := s.search(ctx, keyword)
		if err != nil {
			return domain.MovieMetadata{}, err
		}
		id, err := matchFANZA(identities, code)
		if err != nil {
			return domain.MovieMetadata{}, err
		}
		if id == "" {
			continue
		}
		var response struct {
			Content *fanzaMovie               `json:"ppvContent"`
			Review  struct{ Average float64 } `json:"reviewSummary"`
		}
		if err := s.graphQL(ctx, fanzaDetailQuery, map[string]any{"id": id}, &response); err != nil {
			return domain.MovieMetadata{}, err
		}
		if response.Content == nil {
			return domain.MovieMetadata{}, metadata.ErrNotFound
		}
		m := response.Content
		if m.ID != id || !codeid.IsFormatEquivalent(m.Code, code) || strings.TrimSpace(m.Title) == "" {
			return domain.MovieMetadata{}, fmt.Errorf("FANZA 详情与已确认影片不一致: %s", id)
		}
		return fanzaMetadata(*m, response.Review.Average), nil
	}
	return domain.MovieMetadata{}, metadata.ErrNotFound
}

func (s *FANZA) search(ctx context.Context, keyword string) ([]fanzaIdentity, error) {
	var identities []fanzaIdentity
	for offset := 0; offset < 120; offset += 40 {
		var response struct {
			Search *struct {
				Result *struct {
					Contents []struct{ ID string }
					Page     *struct{ HasNext bool } `json:"pageInfo"`
				}
			} `json:"legacySearchPPV"`
		}
		if err := s.graphQL(ctx, fanzaSearchQuery, map[string]any{"keyword": keyword, "offset": offset}, &response); err != nil {
			return nil, err
		}
		if response.Search == nil || response.Search.Result == nil || response.Search.Result.Page == nil {
			return nil, fmt.Errorf("FANZA 未返回完整检索结果")
		}
		page := response.Search.Result
		if len(page.Contents) > 0 {
			// Batch identity-only lookups avoid downloading every unrelated film's
			// full metadata and avoid guessing studio prefixes from content IDs.
			var declarations, selections []string
			variables := make(map[string]any, len(page.Contents))
			for i, item := range page.Contents {
				name := fmt.Sprintf("id%d", i)
				declarations = append(declarations, "$"+name+": ID!")
				selections = append(selections, fmt.Sprintf("m%d: ppvContent(id: $%s) { id makerContentId }", i, name))
				variables[name] = item.ID
			}
			query := "query MovieIdentities(" + strings.Join(declarations, ",") + ") {" + strings.Join(selections, " ") + "}"
			var matched map[string]*fanzaIdentity
			if err := s.graphQL(ctx, query, variables, &matched); err != nil {
				return nil, err
			}
			for i, item := range page.Contents {
				key := fmt.Sprintf("m%d", i)
				identity, present := matched[key]
				if !present {
					return nil, fmt.Errorf("FANZA 未返回候选身份")
				}
				if identity != nil {
					if identity.ID != item.ID || identity.Code == "" {
						return nil, fmt.Errorf("FANZA 返回了无效候选身份")
					}
					identities = append(identities, *identity)
				}
			}
		}
		if !page.Page.HasNext {
			return identities, nil
		}
	}
	return nil, domain.E(domain.KindConflict, "FANZA 检索候选过多，无法可靠确认影片", nil)
}

func matchFANZA(identities []fanzaIdentity, code string) (string, error) {
	for _, exact := range []bool{true, false} {
		id := ""
		for _, identity := range identities {
			matches := codeid.IsFormatEquivalent(identity.Code, code)
			if exact {
				matches = codeid.Normalize(identity.Code) == code
			}
			if !matches {
				continue
			}
			if id != "" && id != identity.ID {
				return "", domain.E(domain.KindConflict, "FANZA 同番号对应多个商品，无法唯一匹配", nil)
			}
			id = identity.ID
		}
		if id != "" {
			return id, nil
		}
	}
	return "", nil
}

func (s *FANZA) graphQL(ctx context.Context, query string, variables map[string]any, result any) error {
	if err := s.admit(ctx); err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", s.base)
	req.Header.Set("Fanza-Device", "BROWSER")
	req.Header.Set("User-Agent", "")
	response, err := s.http.Do(req)
	if err != nil {
		return s.failed(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return s.failed(&domain.HTTPError{Source: "FANZA", StatusCode: response.StatusCode, RetryAfter: domain.ParseRetryAfter(response.Header.Get("Retry-After"), time.Now())})
	}
	body, err = io.ReadAll(io.LimitReader(response.Body, 4<<20+1))
	if err != nil {
		return s.failed(err)
	}
	if len(body) > 4<<20 {
		return fmt.Errorf("FANZA response exceeds 4 MiB")
	}
	var envelope struct {
		Data   json.RawMessage
		Errors []struct{ Message string }
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode FANZA response: %w", err)
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("FANZA GraphQL: %s", envelope.Errors[0].Message)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return fmt.Errorf("FANZA 未返回查询数据")
	}
	return json.Unmarshal(envelope.Data, result)
}

func fanzaMetadata(source fanzaMovie, rating float64) domain.MovieMetadata {
	m := domain.MovieDetail{Movie: domain.Movie{
		Code: codeid.Normalize(source.Code), Title: source.Title, OriginTitle: source.Title,
		Duration: source.Duration / 60,
		Rating:   rating, RatingMax: 5, RatingSource: "fanza",
		Sources: []domain.SourceID{{Provider: "fanza", ID: source.ID}},
		Cover:   source.PackageImage.LargeURL, Thumbnail: source.PackageImage.MediumURL,
	}}
	if m.Cover == "" {
		m.Cover = m.Thumbnail
	}
	if m.Thumbnail == "" {
		m.Thumbnail = m.Cover
	}
	switch source.Floor {
	case "AV", "AMATEUR":
		m.Zone = domain.ZoneCensored
	case "ANIME":
		m.Zone = domain.ZoneAnime
	default:
		m.Zone = domain.ZoneUnknown
	}
	date := source.MakerReleasedAt
	if date == nil {
		date = source.DeliveryStartDate
	}
	if date != nil {
		m.ReleaseDate = date.In(time.FixedZone("JST", 9*60*60)).Format(time.DateOnly)
	}
	for _, actor := range source.Actresses {
		m.Actors = append(m.Actors, domain.Actor{Provider: "fanza", ID: actor.ID, Name: actor.Name, Avatar: actor.ImageURL, Gender: "female"})
	}
	for _, actor := range source.Histrions {
		m.Actors = append(m.Actors, domain.Actor{Provider: "fanza", ID: actor.ID, Name: actor.Name, Gender: "male"})
	}
	if len(m.Actors) == 0 && source.AmateurActress != nil && source.AmateurActress.Name != "" {
		a := source.AmateurActress
		m.Actors = append(m.Actors, domain.Actor{Provider: "fanza", ID: a.ID, Name: a.Name, Avatar: a.ImageURL, Gender: "unknown"})
	}
	if source.Maker != nil {
		m.Maker = &domain.Maker{Provider: "fanza", ID: source.Maker.ID, Name: source.Maker.Name}
	}
	if source.Series != nil {
		m.Series = &domain.Series{Provider: "fanza", ID: source.Series.ID, Name: source.Series.Name}
	}
	if len(source.Directors) > 0 {
		d := source.Directors[0]
		m.Director = &domain.Director{Provider: "fanza", ID: d.ID, Name: d.Name}
	}
	for _, genre := range source.Genres {
		m.Tags = append(m.Tags, domain.Tag{Provider: "fanza", ID: genre.ID, Name: genre.Name})
	}
	result := domain.MovieMetadata{Detail: m}
	for _, url := range []string{m.Cover, source.PackageImage.MediumURL} {
		if url != "" && (len(result.Images) == 0 || result.Images[0].URL != url) {
			result.Images = append(result.Images, domain.ImageCandidate{Provider: "fanza", URL: url, Role: "cover"})
		}
	}
	for _, image := range source.SampleImages {
		url := image.LargeImageURL
		if url == "" {
			url = image.ImageURL
		}
		if url != "" {
			result.Images = append(result.Images, domain.ImageCandidate{Provider: "fanza", URL: url, Role: "preview"})
			result.Detail.PreviewImages = append(result.Detail.PreviewImages, domain.PreviewImage{Original: url, Thumbnail: image.ImageURL})
		}
	}
	result.Detail.HasPreview = len(result.Detail.PreviewImages) > 0
	return result
}
