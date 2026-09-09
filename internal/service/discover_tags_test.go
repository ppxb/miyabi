package service

import (
	"context"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/javdb"
)

func TestMovieTagsResolveGlobalIDsFromCachedTaxonomies(t *testing.T) {
	service := &DiscoverService{tags: newResponseCache[[]javdb.TagCategory](5, time.Hour)}
	fixtures := map[javdb.Zone][]javdb.TagCategory{
		javdb.ZoneAnime:      {{ID: "anime-category", Tags: []javdb.TagOption{{ID: "anime-tag", Name: "动漫标签"}}}},
		javdb.ZoneCensored:   {{ID: "shared-category", Tags: []javdb.TagOption{{ID: "shared-tag", Name: "共享标签"}}}},
		javdb.ZoneUncensored: {{ID: "other-category", Tags: []javdb.TagOption{{ID: "other-tag", Name: "其他标签"}}}},
		javdb.ZoneWestern:    {},
		javdb.ZoneFC2:        {},
	}
	for zone, categories := range fixtures {
		if _, err := service.tags.get(t.Context(), string(zone), func(context.Context) ([]javdb.TagCategory, error) {
			return categories, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	detail := javdb.MovieDetail{Zone: javdb.ZoneAnime, Movie: javdb.Movie{Tags: []javdb.Tag{
		{ID: "anime-tag"},
		{ID: "shared-tag", Name: "上游名称"},
		{ID: "other-tag"},
		{ID: "shared-tag"},
	}}}
	if err := service.completeMovieTags(t.Context(), &detail); err != nil {
		t.Fatal(err)
	}
	want := []javdb.Tag{
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
	detail.Tags = []javdb.Tag{{ID: "missing-tag"}}
	if err := service.completeMovieTags(t.Context(), &detail); err != nil || len(detail.Tags) != 0 {
		t.Fatalf("unnamed retired tag blocked the movie: %#v, %v", detail.Tags, err)
	}
	detail.Tags = []javdb.Tag{{ID: "named-tag", Name: "上游标签"}}
	if err := service.completeMovieTags(t.Context(), &detail); err != nil || len(detail.Tags) != 1 || detail.Tags[0].Name != "上游标签" {
		t.Fatalf("available tag name was discarded: %#v, %v", detail.Tags, err)
	}
}

func TestCompleteMovieTagsSkipsLookupsForCompleteMetadata(t *testing.T) {
	service := &DiscoverService{}
	for _, tags := range [][]javdb.Tag{nil, {{ID: "known", Name: "已知标签", CategoryID: "known-category"}}} {
		if err := service.completeMovieTags(t.Context(), &javdb.MovieDetail{Movie: javdb.Movie{Tags: tags}}); err != nil {
			t.Fatal(err)
		}
	}
}
