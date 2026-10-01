package metadata

import (
	"reflect"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
)

func TestMergeKeepsJavDBEntitiesAndAllSourceArtwork(t *testing.T) {
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
	catalogue.Detail.Tags[0].CategoryID = "catalogue-category"
	catalogue.Detail.Tags[0].NameZHT = "標籤"
	catalogue.Detail.Series = &domain.Series{Provider: "javdb", ID: "series", Name: "Series"}
	catalogue.Detail.Director = &domain.Director{Provider: "javdb", ID: "director", Name: "Director"}
	catalogue.Images = []domain.ImageCandidate{
		{Provider: "javdb", Role: "cover", URL: "https://catalogue.example/cover.jpg"},
		{Provider: "javdb", Role: "preview", URL: "https://catalogue.example/preview.jpg"},
		{Provider: "javdb", Role: "preview", URL: "https://official.example/preview.jpg"},
	}
	input := []domain.MovieMetadata{official, catalogue}
	got := merge(input)
	if got.Detail.Title != catalogue.Detail.Title || got.Detail.OriginTitle != catalogue.Detail.OriginTitle || got.Detail.Zone != domain.ZoneCensored ||
		got.Detail.Rating != 4.2 || got.Detail.RatingSource != "javdb" {
		t.Fatalf("incorrect field priority: %+v", got.Detail)
	}
	if !reflect.DeepEqual(got.Detail.Actors, catalogue.Detail.Actors) || !reflect.DeepEqual(got.Detail.Tags, catalogue.Detail.Tags) ||
		!reflect.DeepEqual(got.Detail.Maker, catalogue.Detail.Maker) || !reflect.DeepEqual(got.Detail.Series, catalogue.Detail.Series) || !reflect.DeepEqual(got.Detail.Director, catalogue.Detail.Director) {
		t.Fatalf("catalogue entity identities were replaced: %+v", got.Detail)
	}
	if len(got.Images) != 4 || !reflect.DeepEqual(got.Images[:2], official.Images) || len(got.Detail.PreviewImages) != 2 || !got.Detail.HasPreview {
		t.Fatalf("artwork candidates lost or duplicated: %+v", got)
	}
	if input[0].Detail.Sources[0].Provider != "fanza" || official.Detail.Tags[0].Provider != "fanza" {
		t.Fatal("merge mutated source results")
	}
}

func TestMergeSupplementsMissingJavDBFieldsAndPreviews(t *testing.T) {
	catalogue := fixture("javdb", "ABP-123", "Catalogue title")
	official := completeFixture("fanza", "ABP-123")
	got := merge([]domain.MovieMetadata{official, catalogue})
	if got.Detail.Title != "Catalogue title" || got.Detail.Maker.Provider != "fanza" || got.Detail.Actors[0].Provider != "fanza" ||
		len(got.Detail.PreviewImages) != 1 || got.Detail.FieldSources["title"] != "javdb" || got.Detail.FieldSources["tags"] != "fanza" {
		t.Fatalf("missing metadata not supplemented: %+v", got)
	}
	if needsSupplement(got) {
		t.Fatal("complete metadata triggered another query")
	}
}
