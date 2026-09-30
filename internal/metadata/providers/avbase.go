package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/net/html"
)

// AVBase supplies its own work/cast identities plus original product images.
// Reading server-rendered page data avoids coupling requests to Next.js build IDs.
type AVBase struct {
	client
	pages *netx.ProxiedFingerprintClient
}

func NewAVBase(proxy *netx.ProxyManager) (*AVBase, error) {
	pages, err := netx.NewProxiedFingerprintClient(proxy, netx.FingerprintOptions{Timeout: 20 * time.Second, CookieJar: true})
	if err != nil {
		return nil, err
	}
	return &AVBase{client: newClient(proxy, "https://www.avbase.net/", "", "avbase.net", "dmm.co.jp", "dmm.com", "mgstage.com", "mgstage.jp", "duga.jp", "getchu.com", "pcolle.com"), pages: pages}, nil
}
func (*AVBase) ID() string { return "avbase" }
func (*AVBase) Supports(code string) bool {
	return catalogueCode.MatchString(code) && !strings.HasPrefix(code, "FC2-") && !strings.HasPrefix(code, "HEYZO-")
}
func (s *AVBase) Close() { s.pages.Close(); s.http.CloseIdleConnections() }

type avbaseEntity struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	ImageURL string `json:"image_url"`
}
type avbaseProduct struct {
	Source       string       `json:"source"`
	ID           string       `json:"product_id"`
	Title        string       `json:"title"`
	ImageURL     string       `json:"image_url"`
	ThumbnailURL string       `json:"thumbnail_url"`
	Date         string       `json:"date"`
	Maker        avbaseEntity `json:"maker"`
	Series       avbaseEntity `json:"series"`
	Samples      []struct {
		Large string `json:"l"`
	} `json:"sample_image_urls"`
	Info struct {
		Description string `json:"description"`
		Volume      string `json:"volume"`
	} `json:"iteminfo"`
}
type avbaseWork struct {
	ID       int             `json:"id"`
	Prefix   string          `json:"prefix"`
	Code     string          `json:"work_id"`
	Title    string          `json:"title"`
	Date     string          `json:"min_date"`
	Products []avbaseProduct `json:"products"`
	Casts    []struct {
		Actor avbaseEntity `json:"actor"`
	} `json:"casts"`
	Actors []avbaseEntity `json:"actors"`
	Genres []avbaseEntity `json:"genres"`
}

func (work avbaseWork) key() string {
	if work.Prefix != "" {
		return work.Prefix + ":" + work.Code
	}
	return work.Code
}

type avbasePage struct {
	Work  *avbaseWork  `json:"work"`
	Works []avbaseWork `json:"works"`
}

func (s *AVBase) Fetch(ctx context.Context, code string) (domain.MovieMetadata, error) {
	page, err := s.page(ctx, "works/"+url.PathEscape(code))
	if err == nil && page.Work != nil {
		if !codeid.IsFormatEquivalent(page.Work.Code, code) {
			return domain.MovieMetadata{}, fmt.Errorf("AVBase 详情番号 %s 与 %s 不一致", page.Work.Code, code)
		}
		return avbaseMetadata(*page.Work)
	}
	if err != nil && !errors.Is(err, metadata.ErrNotFound) {
		return domain.MovieMetadata{}, err
	}
	page, err = s.page(ctx, "works?q="+url.QueryEscape(code))
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	work, err := matchAVBase(page.Works, code)
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	// Search cards omit cast and product fields; always load the confirmed work.
	page, err = s.page(ctx, "works/"+url.PathEscape(work.key()))
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	if page.Work == nil || page.Work.key() != work.key() {
		return domain.MovieMetadata{}, fmt.Errorf("AVBase 返回了不同的作品身份")
	}
	return avbaseMetadata(*page.Work)
}

func matchAVBase(works []avbaseWork, code string) (avbaseWork, error) {
	for _, equivalent := range []bool{false, true} {
		var result avbaseWork
		for _, work := range works {
			match := codeid.Normalize(work.Code) == codeid.Normalize(code)
			if equivalent {
				match = codeid.IsFormatEquivalent(work.Code, code)
			}
			if !match {
				continue
			}
			if result.Code != "" && result.key() != work.key() {
				return avbaseWork{}, domain.E(domain.KindConflict, "AVBase 存在多个同番号作品，无法唯一匹配", nil)
			}
			result = work
		}
		if result.Code != "" {
			return result, nil
		}
	}
	return avbaseWork{}, metadata.ErrNotFound
}

func (s *AVBase) page(ctx context.Context, relative string) (avbasePage, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return avbasePage{}, err
	}
	select {
	case <-s.pages.Changes():
		if err := s.pages.Refresh(); err != nil {
			return avbasePage{}, err
		}
	default:
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+relative, nil)
	if err != nil {
		return avbasePage{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", s.base)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	response, err := s.pages.Do(req)
	if err != nil {
		return avbasePage{}, err
	}
	defer response.Body.Close()
	if response.StatusCode == 404 {
		return avbasePage{}, metadata.ErrNotFound
	}
	if response.StatusCode != 200 {
		return avbasePage{}, fmt.Errorf("AVBase HTTP %d，未取得作品资料", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil {
		return avbasePage{}, err
	}
	if len(body) > 4<<20 {
		return avbasePage{}, fmt.Errorf("AVBase page exceeds 4 MiB")
	}
	return parseAVBasePage(body)
}

func parseAVBasePage(body []byte) (avbasePage, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return avbasePage{}, err
	}
	script := id(doc, "__NEXT_DATA__")
	if script == nil || script.FirstChild == nil {
		return avbasePage{}, fmt.Errorf("AVBase 页面缺少作品数据，可能遇到站点验证")
	}
	var data struct {
		Props struct {
			Page avbasePage `json:"pageProps"`
		} `json:"props"`
	}
	if err := json.Unmarshal([]byte(script.FirstChild.Data), &data); err != nil {
		return avbasePage{}, fmt.Errorf("decode AVBase page data: %w", err)
	}
	if data.Props.Page.Work == nil && data.Props.Page.Works == nil {
		return avbasePage{}, fmt.Errorf("AVBase 未返回作品或检索结果")
	}
	return data.Props.Page, nil
}

func avbaseMetadata(work avbaseWork) (domain.MovieMetadata, error) {
	m := domain.MovieDetail{Movie: domain.Movie{Code: codeid.Normalize(work.Code), Title: work.Title, ReleaseDate: releaseDate(work.Date), Sources: []domain.SourceID{{Provider: "avbase", ID: work.key()}}}}
	// Stable product ordering keeps metadata deterministic across API ordering changes.
	sort.Slice(work.Products, func(i, j int) bool {
		a, b := work.Products[i], work.Products[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return a.ID < b.ID
	})
	var images []domain.ImageCandidate
	for _, p := range work.Products {
		if m.Title == "" {
			m.Title = p.Title
		}
		if m.Summary == "" {
			m.Summary = p.Info.Description
		}
		if m.ReleaseDate == "" {
			m.ReleaseDate = releaseDate(p.Date)
		}
		if m.Duration == 0 {
			m.Duration = integer(p.Info.Volume)
		}
		if m.Cover == "" {
			m.Cover = p.ImageURL
			m.Thumbnail = p.ImageURL
		}
		if m.Maker == nil && p.Maker.Name != "" {
			m.Maker = &domain.Maker{Provider: "avbase", ID: avbaseID(p.Maker.ID), Name: p.Maker.Name}
		}
		if m.Series == nil && p.Series.Name != "" {
			m.Series = &domain.Series{Provider: "avbase", ID: avbaseID(p.Series.ID), Name: p.Series.Name}
		}
		if p.Source != "" && p.ID != "" {
			m.Sources = append(m.Sources, domain.SourceID{Provider: p.Source, ID: p.ID})
		}
		if p.ImageURL != "" {
			images = append(images, domain.ImageCandidate{Provider: "avbase", URL: p.ImageURL, Role: "cover"})
		}
		for _, sample := range p.Samples {
			if sample.Large != "" {
				images = append(images, domain.ImageCandidate{Provider: "avbase", URL: sample.Large, Role: "preview"})
			}
		}
	}
	actors := make(map[int]bool)
	for _, cast := range work.Casts {
		work.Actors = append(work.Actors, cast.Actor)
	}
	for _, actor := range work.Actors {
		if actor.ID == 0 || actor.Name == "" || actors[actor.ID] {
			continue
		}
		actors[actor.ID] = true
		m.Actors = append(m.Actors, domain.Actor{Provider: "avbase", ID: strconv.Itoa(actor.ID), Name: actor.Name, Avatar: actor.ImageURL})
	}
	for _, genre := range work.Genres {
		m.Tags = append(m.Tags, domain.Tag{Provider: "avbase", ID: avbaseID(genre.ID), Name: genre.Name})
	}
	if m.Code == "" || m.Title == "" {
		return domain.MovieMetadata{}, fmt.Errorf("AVBase 作品资料缺少番号或标题")
	}
	return domain.MovieMetadata{Detail: m, Images: images}, nil
}
func avbaseID(id int) string {
	if id == 0 {
		return ""
	}
	return strconv.Itoa(id)
}
