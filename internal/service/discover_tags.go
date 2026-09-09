package service

import (
	"context"

	"github.com/ppxb/miyabi/internal/javdb"
)

// Catalogue tag IDs are global. Some details include unnamed tags whose
// definitions live in another section's taxonomy; complete them before caching.
func (service *DiscoverService) completeMovieTags(ctx context.Context, detail *javdb.MovieDetail) error {
	var missing []int
	for index, tag := range detail.Tags {
		if tag.Name == "" || tag.CategoryID == "" {
			missing = append(missing, index)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	zones := []javdb.Zone{detail.Zone, javdb.ZoneCensored, javdb.ZoneUncensored, javdb.ZoneWestern, javdb.ZoneFC2, javdb.ZoneAnime}
	for index, zone := range zones {
		if index > 0 && zone == detail.Zone {
			continue
		}
		categories, err := service.Tags(ctx, zone)
		if err != nil {
			return err
		}
		definitions := make(map[string]javdb.Tag)
		for _, category := range categories {
			for _, tag := range category.Tags {
				definitions[tag.ID] = javdb.Tag{Name: tag.Name, CategoryID: category.ID}
			}
		}
		pending := missing[:0]
		for _, index := range missing {
			tag := &detail.Tags[index]
			definition, found := definitions[tag.ID]
			if found {
				if tag.Name == "" {
					tag.Name = definition.Name
				}
				if tag.CategoryID == "" {
					tag.CategoryID = definition.CategoryID
				}
			}
			if tag.Name == "" || tag.CategoryID == "" {
				pending = append(pending, index)
			}
		}
		missing = pending
		if len(missing) == 0 {
			return nil
		}
	}
	// Retired tag IDs can remain on a movie without a name or taxonomy entry.
	// Keep available metadata; an unnamed optional tag cannot be displayed.
	tags := detail.Tags[:0]
	for _, tag := range detail.Tags {
		if tag.Name != "" {
			tags = append(tags, tag)
		}
	}
	detail.Tags = tags
	return nil
}
