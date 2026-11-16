package javdb

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

var zoneCodes = map[domain.Zone]int{
	domain.ZoneCensored:   0,
	domain.ZoneUncensored: 1,
	domain.ZoneWestern:    2,
	domain.ZoneFC2:        3,
	domain.ZoneAnime:      4,
}

var webZoneFilters = map[domain.Zone]string{
	domain.ZoneCensored:   "censored",
	domain.ZoneUncensored: "uncensored",
	domain.ZoneWestern:    "western",
	domain.ZoneFC2:        "fc2",
	domain.ZoneAnime:      "anime",
}

// Search searches JavDB's movie catalogue. The App API asks for an account,
// so an unconfigured client and a rejected API both fall back to the anonymous
// public search page.
func (c *Client) Search(ctx context.Context, keyword string, options domain.SearchOptions) ([]domain.Movie, error) {
	params, err := buildSearchParams(keyword, options)
	if err != nil {
		return nil, err
	}
	if username, password := c.Credentials(); username == "" || password == "" {
		return c.searchFromWeb(ctx, keyword, options)
	}
	var data wireMoviesData
	if err := c.getJSONWithAuth(ctx, "/api/v2/search", params, &data); err != nil {
		if requiresWebFallback(err) {
			return c.searchFromWeb(ctx, keyword, options)
		}
		return nil, err
	}
	return moviesFromWire(ctx, data.Movies)
}

func (c *Client) searchFromWeb(ctx context.Context, keyword string, options domain.SearchOptions) ([]domain.Movie, error) {
	if c.webSearchLimiter == nil {
		c.webSearchLimiter = rate.NewLimiter(rate.Every(webSearchInterval), 1)
	}
	if err := c.webSearchLimiter.Wait(ctx); err != nil {
		return nil, err
	}
	page := options.Page
	if page <= 0 {
		page = 1
	}
	params := url.Values{
		"q":    {strings.TrimSpace(keyword)},
		"f":    {webZoneFilters[options.Zone]},
		"page": {strconv.Itoa(page)},
	}
	if params.Get("f") == "" {
		params.Set("f", "all")
	}
	body, err := c.fetchWebHTML(ctx, "/search", params)
	if err != nil {
		return nil, err
	}
	if webLoginTitlePattern.Match(body) {
		return nil, ErrLoginRequired
	}
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	return parseWebSearch(document, options.Limit), nil
}

func buildSearchParams(keyword string, options domain.SearchOptions) (url.Values, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, errors.New("JavDB search keyword is required")
	}
	if options.Page <= 0 {
		options.Page = 1
	}

	params := url.Values{
		"q":    {keyword},
		"page": {strconv.Itoa(options.Page)},
		"type": {"movie"},
	}
	if options.Limit > 0 {
		params.Set("limit", strconv.Itoa(options.Limit))
	}
	if options.Sort != "" {
		params.Set("movie_sort_by", options.Sort)
	}
	if options.FilterBy != "" {
		params.Set("movie_filter_by", options.FilterBy)
	}

	zone := options.Zone
	if zone == "" {
		zone = domain.ZoneAll
	}
	if zone != domain.ZoneAll {
		code, ok := zoneCodes[zone]
		if !ok {
			return nil, errors.New("JavDB zone must be censored, uncensored, western, fc2, anime, or all")
		}
		params.Set("movie_type", strconv.Itoa(code))
	}
	return params, nil
}
