package javdb

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
)

var zoneCodes = map[domain.Zone]int{
	domain.ZoneCensored:   0,
	domain.ZoneUncensored: 1,
	domain.ZoneWestern:    2,
	domain.ZoneFC2:        3,
	domain.ZoneAnime:      4,
}

// Search searches JavDB's movie catalogue.
func (c *Client) Search(ctx context.Context, keyword string, options domain.SearchOptions) ([]domain.Movie, error) {
	params, err := buildSearchParams(keyword, options)
	if err != nil {
		return nil, err
	}

	var data wireMoviesData
	if err := c.getJSON(ctx, "/api/v2/search", params, &data); err != nil {
		return nil, err
	}
	return moviesFromWire(ctx, data.Movies)
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
