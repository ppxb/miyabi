package catalogue

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

func TestMovieTagsResolveGlobalIDsFromCachedTaxonomies(t *testing.T) {
	service := &Service{tags: newResponseCache[[]domain.TagCategory](5, time.Hour)}
	fixtures := map[domain.Zone][]domain.TagCategory{
		domain.ZoneAnime:      {{ID: "anime-category", Tags: []domain.TagOption{{ID: "anime-tag", Name: "动漫标签"}}}},
		domain.ZoneCensored:   {{ID: "shared-category", Tags: []domain.TagOption{{ID: "shared-tag", Name: "共享标签"}}}},
		domain.ZoneUncensored: {{ID: "other-category", Tags: []domain.TagOption{{ID: "other-tag", Name: "其他标签"}}}},
		domain.ZoneWestern:    {},
		domain.ZoneFC2:        {},
	}
	for zone, categories := range fixtures {
		if _, err := service.tags.get(t.Context(), string(zone), func(context.Context) ([]domain.TagCategory, time.Duration, error) {
			return categories, 0, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	detail := domain.MovieDetail{Zone: domain.ZoneAnime, Movie: domain.Movie{Tags: []domain.Tag{
		{ID: "anime-tag"},
		{ID: "shared-tag", Name: "上游名称"},
		{ID: "other-tag"},
		{ID: "shared-tag"},
	}}}
	if err := service.completeMovieTags(t.Context(), &detail); err != nil {
		t.Fatal(err)
	}
	want := []domain.Tag{
		{ID: "anime-tag", Name: "动漫标签", CategoryID: "anime-category"},
		{ID: "shared-tag", Name: "上游名称", CategoryID: "shared-category"},
		{ID: "other-tag", Name: "其他标签", CategoryID: "other-category"},
		{ID: "shared-tag", Name: "共享标签", CategoryID: "shared-category"},
	}
	for index, tag := range detail.Tags {
		if tag != want[index] {
			t.Fatalf("tag %d = %#v, want %#v", index, tag, want[index])
		}
	}
	detail.Tags = []domain.Tag{{ID: "missing-tag"}}
	if err := service.completeMovieTags(t.Context(), &detail); err != nil || len(detail.Tags) != 0 {
		t.Fatalf("unnamed retired tag blocked the movie: %#v, %v", detail.Tags, err)
	}
	detail.Tags = []domain.Tag{{ID: "named-tag", Name: "上游标签"}}
	if err := service.completeMovieTags(t.Context(), &detail); err != nil || len(detail.Tags) != 1 || detail.Tags[0].Name != "上游标签" {
		t.Fatalf("available tag name was discarded: %#v, %v", detail.Tags, err)
	}
}

func TestCompleteMovieTagsSkipsLookupsForCompleteMetadata(t *testing.T) {
	service := &Service{}
	for _, tags := range [][]domain.Tag{nil, {{ID: "known", Name: "已知标签", CategoryID: "known-category"}}} {
		detail := domain.MovieDetail{Zone: domain.ZoneCensored, Movie: domain.Movie{Tags: tags}}
		if err := service.completeMovieTags(t.Context(), &detail); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(detail.Tags, tags) {
			t.Fatalf("tags changed: %#v", detail.Tags)
		}
	}
}

func TestCompleteMovieTagsUnknownZoneKeepsNamedTagsOnly(t *testing.T) {
	service := &Service{}
	detail := domain.MovieDetail{
		Zone: domain.ZoneUnknown,
		Movie: domain.Movie{
			Tags: []domain.Tag{
				{ID: "tag-1", Name: "Named Tag"},
				{ID: "tag-2"},
			},
		},
	}
	if err := service.completeMovieTags(t.Context(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Tags) != 1 || detail.Tags[0].ID != "tag-1" {
		t.Fatalf("unexpected tags for unknown zone: %#v", detail.Tags)
	}
}
