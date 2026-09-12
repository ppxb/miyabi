package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
)

func TestLibraryPageLoadsCardMetadataWithScopedCounts(t *testing.T) {
	library, _, payload := libraryFixture(t)
	ctx := t.Context()
	database := library.database
	tagB := database.Tag.Create().SetJavdbID("tag-b").SetName("B tag").SetCategoryID("category").SaveX(ctx)
	tagA := database.Tag.Create().SetJavdbID("tag-a").SetName("A tag").SetCategoryID("category").SaveX(ctx)
	actorB := database.Actor.Create().SetJavdbID("actor-b").SetName("B actor").SaveX(ctx)
	actorA := database.Actor.Create().SetJavdbID("actor-a").SetName("A actor").SaveX(ctx)
	releaseDate := time.Date(2026, time.September, 12, 0, 0, 0, 0, time.UTC)
	film := database.Movie.Create().SetCode("ABP-001").SetTitle("Fixture title").SetCover("/cover").
		SetFanarts([]string{"/api/library/artwork/fanart"}).SetReleaseDate(releaseDate).SetDuration(125).SetRating(4.5).
		SetMakerID("maker-id").SetMakerName("Studio").SetSeriesID("series-id").SetSeriesName("Series").
		SetDirectorID("director-id").SetDirectorName("Director").AddActors(actorB, actorA).
		SetScrapeStatus(movie.ScrapeStatusDone).SetWatched(true).AddTags(tagB, tagA).SaveX(ctx)
	empty := database.Movie.Create().SetCode("ABP-002").SetMakerName("Legacy studio").SaveX(ctx)
	hidden := database.Movie.Create().SetCode("ABP-003").SetTitle("Other source").AddTags(tagB).SaveX(ctx)
	for index, entry := range []struct {
		movieID int
		account string
		root    string
	}{
		{film.ID, "100", "10"}, {film.ID, "100", "10"}, {empty.ID, "100", "10"},
		{0, "100", "10"}, {hidden.ID, "other", "10"}, {hidden.ID, "100", "other"},
		{0, "other", "10"}, {0, "100", "other"},
	} {
		builder := database.File.Create().SetFileID(fmt.Sprint(index)).SetName("video.mp4").SetSize(1024).
			SetAccountID(entry.account).SetRootID(entry.root)
		if entry.movieID != 0 {
			builder.SetMovieID(entry.movieID)
		}
		builder.ExecX(ctx)
	}
	queries := make(map[string]int)
	database.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			queries[fmt.Sprintf("%T", query)]++
			return next.Query(ctx, query)
		})
	}))
	page, err := library.Movies(ctx, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if page.Source == nil || *page.Source != payload.Source || page.Total != 2 || page.HasMore || len(page.Movies) != 2 {
		t.Fatalf("library statistics escaped the mounted source: %#v", page)
	}
	wantQueries := map[string]int{"*ent.SettingQuery": 1, "*ent.FileQuery": 1, "*ent.MovieQuery": 1, "*ent.TagQuery": 1, "*ent.ActorQuery": 1}
	if !reflect.DeepEqual(queries, wantQueries) {
		t.Fatalf("library cards made redundant queries: %#v", queries)
	}
	for _, item := range page.Movies {
		if item.ID == film.ID {
			if item.Title != film.Title || item.Cover == nil || *item.Cover != "/cover" ||
				item.ScrapeStatus != movie.ScrapeStatusDone || !item.Watched ||
				!reflect.DeepEqual(item.Tags, []LibraryTag{{ID: tagA.ID, JavDBID: tagA.JavdbID, Name: tagA.Name}, {ID: tagB.ID, JavDBID: tagB.JavdbID, Name: tagB.Name}}) {
				t.Fatalf("card lost or reordered catalogue data: %#v", item)
			}
			if item.ReleaseDate != "2026-09-12" || item.Duration != 125 || item.Rating != 4.5 || item.Fanart != "/api/library/artwork/fanart" ||
				!reflect.DeepEqual(item.Maker, &LibraryEntity{ID: "maker-id", Name: "Studio"}) ||
				!reflect.DeepEqual(item.Series, &LibraryEntity{ID: "series-id", Name: "Series"}) ||
				!reflect.DeepEqual(item.Director, &LibraryEntity{ID: "director-id", Name: "Director"}) ||
				!reflect.DeepEqual(item.Actors, []LibraryEntity{{ID: actorA.JavdbID, Name: actorA.Name}, {ID: actorB.JavdbID, Name: actorB.Name}}) {
				t.Fatalf("local hover details were omitted or lost their search IDs: %#v", item)
			}
		} else if item.ID != empty.ID || item.Title != "" || item.Code != empty.Code || item.Tags == nil || len(item.Tags) != 0 ||
			item.Actors == nil || len(item.Actors) != 0 || item.ScrapeStatus != movie.ScrapeStatusPending || item.Watched ||
			!reflect.DeepEqual(item.Maker, &LibraryEntity{Name: "Legacy studio"}) {
			t.Fatalf("unscraped card is not usable: %#v", item)
		}
		body, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			t.Fatal(err)
		}
		for _, removed := range []string{"file_count", "size"} {
			if _, exists := fields[removed]; exists {
				t.Errorf("card still exposes unused field %s", removed)
			}
		}
		for _, required := range []string{"scrape_status", "watched"} {
			if _, exists := fields[required]; !exists {
				t.Errorf("card omits badge field %s", required)
			}
		}
	}
	first, err := library.Movies(ctx, 1, 1)
	if err != nil || len(first.Movies) != 1 || !first.HasMore {
		t.Fatalf("first page: %#v, %v", first, err)
	}
	second, err := library.Movies(ctx, 2, 1)
	if err != nil || len(second.Movies) != 1 || second.HasMore || first.Movies[0].ID == second.Movies[0].ID {
		t.Fatalf("second page: %#v, %v", second, err)
	}
	if beyond, err := library.Movies(ctx, 3, 1); err != nil || beyond.Movies == nil || len(beyond.Movies) != 0 || beyond.HasMore {
		t.Fatalf("out-of-range page: %#v, %v", beyond, err)
	}
}

func TestLibraryPagesContainTwentyDistinctMoviesAndTheRemainder(t *testing.T) {
	library, _, _ := libraryFixture(t)
	ctx := t.Context()
	created := time.Date(2026, time.September, 12, 0, 0, 0, 0, time.UTC)
	for index := range 21 {
		film := library.database.Movie.Create().SetCode(fmt.Sprintf("PAGE-%03d", index)).SetCreatedAt(created).SaveX(ctx)
		for part := range 2 {
			library.database.File.Create().SetFileID(fmt.Sprintf("%d-%d", index, part)).SetName("video.mp4").SetSize(1024).
				SetAccountID("100").SetRootID("10").SetMovieID(film.ID).ExecX(ctx)
		}
	}
	seen := make(map[int]bool)
	previousID := 0
	for pageNumber, count := range []int{20, 1, 0} {
		page, err := library.Movies(ctx, pageNumber+1, 20)
		if err != nil {
			t.Fatal(err)
		}
		if page.Page != pageNumber+1 || page.Total != 21 || len(page.Movies) != count || page.HasMore != (pageNumber == 0) {
			t.Fatalf("incorrect page %d: %#v", pageNumber+1, page)
		}
		for _, film := range page.Movies {
			if seen[film.ID] || (previousID != 0 && film.ID >= previousID) {
				t.Fatalf("pagination repeated or reordered movie %d", film.ID)
			}
			seen[film.ID], previousID = true, film.ID
		}
	}
	if len(seen) != 21 {
		t.Fatalf("pagination omitted movies: %d", len(seen))
	}
}

func TestLibraryPageKeepsEmptyAndUnmatchedSourcesUsable(t *testing.T) {
	library, _, _ := libraryFixture(t)
	for _, unmatched := range []bool{false, true} {
		if unmatched {
			library.database.File.Create().SetFileID("unmatched").SetName("recording.mp4").SetSize(1024).
				SetAccountID("100").SetRootID("10").ExecX(t.Context())
		}
		page, err := library.Movies(t.Context(), 1, 24)
		if err != nil || page.Total != 0 || page.Movies == nil || len(page.Movies) != 0 || page.HasMore {
			t.Fatalf("empty/unmatched source: %#v, %v", page, err)
		}
	}
}
