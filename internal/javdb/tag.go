package javdb

import (
	"context"
	"errors"
	"net/url"
	"strconv"
)

// Tags fetches the Traditional Chinese taxonomy.
func (c *Client) Tags(ctx context.Context, zone Zone) ([]TagCategory, error) {
	code, ok := zoneCodes[zone]
	if !ok {
		return nil, errors.New("JavDB tag zone must be censored, uncensored, western, or fc2")
	}

	var data wireTagsData
	if err := c.getJSON(
		ctx,
		"/api/v2/tags",
		url.Values{"type": {strconv.Itoa(code)}},
		defaultLanguage,
		&data,
	); err != nil {
		return nil, err
	}
	categories := make([]TagCategory, len(data.Tags))
	for categoryIndex, category := range data.Tags {
		result := TagCategory{
			ID:   category.ID,
			Name: category.Name,
			Tags: make([]TagOption, len(category.Tags)),
		}
		for tagIndex, tag := range category.Tags {
			result.Tags[tagIndex] = TagOption{ID: tag.ID, Name: tag.Name}
		}
		categories[categoryIndex] = result
	}
	return categories, nil
}
