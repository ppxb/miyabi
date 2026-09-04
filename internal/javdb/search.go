package javdb

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

var zoneCodes = map[Zone]int{
	ZoneCensored:   0,
	ZoneUncensored: 1,
	ZoneWestern:    2,
	ZoneFC2:        3,
}

// Search searches JavDB's movie catalogue.
func (c *Client) Search(ctx context.Context, keyword string, options SearchOptions) ([]Movie, error) {
	params, err := buildSearchParams(keyword, options)
	if err != nil {
		return nil, err
	}

	var data wireMoviesData
	if err := c.getJSON(ctx, "/api/v2/search", params, defaultLanguage, &data); err != nil {
		return nil, err
	}
	return moviesFromWire(data.Movies)
}

func buildSearchParams(keyword string, options SearchOptions) (url.Values, error) {
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
		zone = ZoneAll
	}
	if zone != ZoneAll {
		code, ok := zoneCodes[zone]
		if !ok {
			return nil, errors.New("JavDB zone must be censored, uncensored, western, fc2, or all")
		}
		params.Set("movie_type", strconv.Itoa(code))
	}
	return params, nil
}
