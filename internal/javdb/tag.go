package javdb

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	"github.com/ppxb/miyabi/internal/domain"
)

// Tags fetches the Traditional Chinese taxonomy.
func (c *Client) Tags(ctx context.Context, zone domain.Zone) ([]domain.TagCategory, error) {
	code, ok := zoneCodes[zone]
	if !ok {
		return nil, errors.New("JavDB tag zone must be censored, uncensored, western, fc2, or anime")
	}

	var data wireTagsData
	if err := c.getJSON(
		ctx,
		"/api/v2/tags",
		url.Values{"type": {strconv.Itoa(code)}},
		&data,
	); err != nil {
		return nil, err
	}
	categories := make([]domain.TagCategory, len(data.Tags))
	for categoryIndex, category := range data.Tags {
		result := domain.TagCategory{
			ID:   category.ID,
			Name: category.Name,
			Tags: make([]domain.TagOption, len(category.Tags)),
		}
		for tagIndex, tag := range category.Tags {
			result.Tags[tagIndex] = domain.TagOption{ID: tag.ID, Name: tag.Name}
		}
		categories[categoryIndex] = result
	}
	return categories, nil
}
