package javdb

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

// Tags fetches and merges the English and Traditional Chinese taxonomy by ID.
func (c *Client) Tags(ctx context.Context, zone Zone) ([]TagCategory, error) {
	english, err := c.fetchTags(ctx, zone, "en")
	if err != nil {
		return nil, err
	}
	traditionalChinese, err := c.fetchTags(ctx, zone, defaultLanguage)
	if err != nil {
		return nil, err
	}
	return mergeTagTaxonomy(english, traditionalChinese)
}

func (c *Client) fetchTags(ctx context.Context, zone Zone, language string) ([]wireTagCategory, error) {
	code, ok := zoneCodes[zone]
	if !ok {
		return nil, errors.New("JavDB tag zone must be censored, uncensored, western, or fc2")
	}

	var data wireTagsData
	if err := c.getJSON(
		ctx,
		"/api/v2/tags",
		url.Values{"type": {strconv.Itoa(code)}},
		language,
		&data,
	); err != nil {
		return nil, err
	}
	return data.Tags, nil
}

func mergeTagTaxonomy(english, traditionalChinese []wireTagCategory) ([]TagCategory, error) {
	zhCategories := make(map[string]wireTagCategory, len(traditionalChinese))
	for _, category := range traditionalChinese {
		zhCategories[category.ID] = category
	}

	categories := make([]TagCategory, len(english))
	for categoryIndex, category := range english {
		translated, ok := zhCategories[category.ID]
		if !ok {
			return nil, fmt.Errorf("JavDB taxonomy category %s has no zh-TW translation", category.ID)
		}
		zhTags := make(map[string]wireTag, len(translated.Tags))
		for _, tag := range translated.Tags {
			zhTags[tag.ID] = tag
		}

		result := TagCategory{
			ID:      category.ID,
			Name:    category.Name,
			NameZHT: translated.Name,
			Tags:    make([]Tag, len(category.Tags)),
		}
		for tagIndex, tag := range category.Tags {
			translatedTag, ok := zhTags[tag.ID]
			if !ok {
				return nil, fmt.Errorf("JavDB taxonomy tag %s has no zh-TW translation", tag.ID)
			}
			result.Tags[tagIndex] = Tag{
				ID:         tag.ID,
				Name:       tag.Name,
				NameZHT:    translatedTag.Name,
				CategoryID: category.ID,
			}
		}
		categories[categoryIndex] = result
	}
	return categories, nil
}
