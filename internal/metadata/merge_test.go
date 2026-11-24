package metadata

import (
	"reflect"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
)

func TestMergeFillsFieldsBySourceOrder(t *testing.T) {
	official := completeFixture("fanza", "ABP-123")
	official.Detail.Zone = domain.ZoneUnknown
	official.Detail.Rating, official.Detail.RatingMax = 4.8, 5
	official.Detail.Tags = append(official.Detail.Tags, domain.Tag{Provider: "fanza", ID: "official-only", Name: "Official genre"})
	official.Images = []domain.ImageCandidate{
		{Provider: "fanza", Role: "cover", URL: "https://official.example/large.jpg"},
		{Provider: "fanza", Role: "preview", URL: "https://official.example/preview.jpg"},
	}
	catalogue := completeFixture("javdb", "ABP-123")
	catalogue.Detail.Zone = domain.ZoneCensored
	catalogue.Detail.OriginTitle = "Original title"
	catalogue.Detail.Rating, catalogue.Detail.RatingMax = 4.2, 5
	catalogue.Detail.Series = &domain.Series{Provider: "javdb", ID: "series", Name: "Series"}
	catalogue.Detail.Director = &domain.Director{Provider: "javdb", ID: "director", Name: "Director"}
	catalogue.Images = []domain.ImageCandidate{
		{Provider: "javdb", Role: "cover", URL: "https://catalogue.example/cover.jpg"},
		{Provider: "javdb", Role: "preview", URL: "https://catalogue.example/preview.jpg"},
		{Provider: "javdb", Role: "preview", URL: "https://official.example/preview.jpg"},
	}
	input := []domain.MovieMetadata{official, catalogue}
	got := merge(input)
	// Source order wins: the first source (fanza) fills the fields it has; javdb
	// only supplements what fanza leaves empty (origin title, series, director).
	if got.Detail.Title != "fanza title" || got.Detail.OriginTitle != "Original title" ||
		got.Detail.Zone != domain.ZoneCensored || got.Detail.Rating != 4.8 || got.Detail.RatingSource != "fanza" {
		t.Fatalf("incorrect field priority: %+v", got.Detail)
	}
	if !reflect.DeepEqual(got.Detail.Actors, official.Detail.Actors) || !reflect.DeepEqual(got.Detail.Tags, official.Detail.Tags) ||
		!reflect.DeepEqual(got.Detail.Maker, official.Detail.Maker) || !reflect.DeepEqual(got.Detail.Series, catalogue.Detail.Series) || !reflect.DeepEqual(got.Detail.Director, catalogue.Detail.Director) {
		t.Fatalf("entity identities not merged by source order: %+v", got.Detail)
	}
	if len(got.Images) != 4 || !reflect.DeepEqual(got.Images[:2], official.Images) || len(got.Detail.PreviewImages) != 2 || !got.Detail.HasPreview {
		t.Fatalf("artwork candidates lost or duplicated: %+v", got)
	}
	if input[0].Detail.Sources[0].Provider != "fanza" || official.Detail.Tags[0].Provider != "fanza" {
		t.Fatal("merge mutated source results")
	}
}

func TestMergePrefersOfficialPlotOverJavDB(t *testing.T) {
	official := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: domain.Movie{
		Sources: []domain.SourceID{{Provider: "heyzo", ID: "1234"}},
		Code:    "HEYZO-1234", Title: "T", Plot: "official plot",
	}}}
	catalogue := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: domain.Movie{
		Sources: []domain.SourceID{{Provider: "javdb", ID: "x"}},
		Code:    "HEYZO-1234", Title: "T", Plot: "javdb plot",
	}}}
	got := merge([]domain.MovieMetadata{official, catalogue})
	if got.Detail.Plot != "official plot" || got.Detail.FieldSources["plot"] != "heyzo" {
		t.Fatalf("plot priority = %q/%q", got.Detail.Plot, got.Detail.FieldSources["plot"])
	}
	official.Detail.Plot = ""
	got = merge([]domain.MovieMetadata{official, catalogue})
	if got.Detail.Plot != "javdb plot" || got.Detail.FieldSources["plot"] != "javdb" {
		t.Fatalf("javdb plot fallback = %q/%q", got.Detail.Plot, got.Detail.FieldSources["plot"])
	}
}

func TestMergeOfficialFillsBeforeJavDBSupplements(t *testing.T) {
	catalogue := fixture("javdb", "ABP-123", "Catalogue title")
	official := completeFixture("fanza", "ABP-123")
	got := merge([]domain.MovieMetadata{official, catalogue})
	if got.Detail.Title != "fanza title" || got.Detail.Maker.Provider != "fanza" || got.Detail.Actors[0].Provider != "fanza" ||
		len(got.Detail.PreviewImages) != 1 || got.Detail.FieldSources["title"] != "fanza" || got.Detail.FieldSources["tags"] != "fanza" {
		t.Fatalf("official fields did not win: %+v", got)
	}
}

func TestMergeTitlePrefersOriginTitle(t *testing.T) {
	source := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: domain.Movie{
		Sources: []domain.SourceID{{Provider: "x", ID: "1"}},
		Code:    "ABP-1", Title: "本地化标题", OriginTitle: "日本語タイトル",
	}}}
	got := merge([]domain.MovieMetadata{source})
	if got.Detail.Title != "日本語タイトル" || got.Detail.OriginTitle != "日本語タイトル" || got.Detail.FieldSources["title"] != "x" {
		t.Fatalf("title should prefer the origin title: title=%q origin=%q src=%q", got.Detail.Title, got.Detail.OriginTitle, got.Detail.FieldSources["title"])
	}
}
