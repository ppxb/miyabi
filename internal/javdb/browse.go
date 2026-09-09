package javdb

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Browse returns one JavDB movie page using the App API tag filter mask.
func (c *Client) Browse(ctx context.Context, options BrowseOptions) ([]Movie, error) {
	params, err := buildBrowseParams(options)
	if err != nil {
		return nil, err
	}

	var data wireMoviesData
	if err := c.getJSON(ctx, "/api/v1/movies/tags", params, defaultLanguage, &data); err != nil {
		return nil, err
	}
	return moviesFromWire(data.Movies)
}

func buildBrowseParams(options BrowseOptions) (url.Values, error) {
	if options.Page <= 0 || options.Limit <= 0 {
		return nil, errors.New("JavDB browse page and limit must be positive")
	}
	if options.Sort == "" || options.Order == "" {
		return nil, errors.New("JavDB browse sort and order are required")
	}

	var filter string
	if options.EntityType != "" || options.EntityID != "" {
		if options.Zone != "" || len(options.TagIDs) != 0 || options.Year != "" || options.Month != "" {
			return nil, errors.New("JavDB entity filters cannot include zone, tags, year, or month")
		}
		letter, ok := map[EntityType]string{
			EntityActor: "a", EntitySeries: "s", EntityMaker: "m", EntityDirector: "d",
		}[options.EntityType]
		if !ok || options.EntityID == "" {
			return nil, errors.New("JavDB entity type and ID are required")
		}
		// Keep the empty leading slot: removing its colon makes JavDB return
		// an unfiltered browse page instead of this entity's movies.
		filter = fmt.Sprintf(":%s:%s", letter, options.EntityID)
		if len(options.Main) > 0 {
			filter += ":" + strings.Join(options.Main, ",") + "::"
		}
	} else {
		zone := ""
		if options.Zone != "" {
			code, ok := zoneCodes[options.Zone]
			if !ok {
				return nil, errors.New("JavDB browse zone must be censored, uncensored, western, fc2, or anime")
			}
			zone = strconv.Itoa(code)
		}
		filter = fmt.Sprintf(
			"%s:t:%s:%s:%s:%s:",
			zone,
			strings.Join(options.Main, ","),
			strings.Join(options.TagIDs, ","),
			options.Year,
			options.Month,
		)
	}
	return url.Values{
		"filter_by": {filter},
		"sort_by":   {options.Sort},
		"order_by":  {options.Order},
		"page":      {strconv.Itoa(options.Page)},
		"limit":     {strconv.Itoa(options.Limit)},
	}, nil
}
