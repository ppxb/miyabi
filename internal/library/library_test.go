package library

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
	lib, _, payload := libraryFixture(t)
	ctx := t.Context()
	database := lib.database
	tagB := database.Tag.Create().SetJavdbID("tag-b").SetName("B tag").SetCategoryID("category").SaveX(ctx)
	tagA := database.Tag.Create().SetJavdbID("tag-a").SetName("A tag").SetCategoryID("category").SaveX(ctx)
	actorB := database.Actor.Create().SetJavdbID("actor-b").SetName("B actor").SaveX(ctx)
	actorA := database.Actor.Create().SetJavdbID("actor-a").SetName("A actor").SaveX(ctx)
	releaseDate := time.Date(2026, time.September, 12, 0, 0, 0, 0, time.UTC)
	film := database.Movie.Create().SetCode("ABP-001").SetTitle("Fixture title").SetCover("/cover").
		SetFanarts([]string{"/api/library/artwork/fanart"}).SetReleaseDate(releaseDate).SetDuration(125).SetRating(4.5).
		SetMakerID("maker-id").SetMakerName("Studio").SetSeriesID("series-id").SetSeriesName("Series").
		SetDirectorID("director-id").SetDirectorName("Director").AddActors(actorB, actorA).
		SetScrapeStatus(movie.ScrapeStatusDone).AddTags(tagB, tagA).SaveX(ctx)
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
	page, err := lib.Movies(ctx, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if page.Source == nil || *page.Source != payload.Source || page.Total != 2 || page.HasMore || len(page.Movies) != 2 {
		t.Fatalf("library statistics escaped the mounted source: %#v", page)
	}
	wantQueries := map[string]int{"*ent.FileQuery": 1, "*ent.MovieQuery": 1, "*ent.TagQuery": 1, "*ent.ActorQuery": 1}
	if !reflect.DeepEqual(queries, wantQueries) {
		t.Fatalf("library cards made redundant queries: %#v", queries)
	}
	for _, item := range page.Movies {
		if item.ID == film.ID {
			if item.Title != film.Title || item.Cover == nil || *item.Cover != "/cover" ||
				item.ScrapeStatus != movie.ScrapeStatusDone ||
				!reflect.DeepEqual(item.Tags, []Tag{{ID: tagA.ID, JavDBID: tagA.JavdbID, Name: tagA.Name}, {ID: tagB.ID, JavDBID: tagB.JavdbID, Name: tagB.Name}}) {
				t.Fatalf("card lost or reordered catalogue data: %#v", item)
			}
			if item.ReleaseDate != "2026-09-12" || item.Duration != 125 || item.Rating != 4.5 || item.Fanart != "/api/library/artwork/fanart" ||
				!reflect.DeepEqual(item.Maker, &Entity{ID: "maker-id", Name: "Studio"}) ||
				!reflect.DeepEqual(item.Series, &Entity{ID: "series-id", Name: "Series"}) ||
				!reflect.DeepEqual(item.Director, &Entity{ID: "director-id", Name: "Director"}) ||
				!reflect.DeepEqual(item.Actors, []Entity{{ID: actorA.JavdbID, Name: actorA.Name}, {ID: actorB.JavdbID, Name: actorB.Name}}) {
				t.Fatalf("local hover details were omitted or lost their search IDs: %#v", item)
			}
		} else if item.ID != empty.ID || item.Title != "" || item.Code != empty.Code || item.Tags == nil || len(item.Tags) != 0 ||
			item.Actors == nil || len(item.Actors) != 0 || item.ScrapeStatus != movie.ScrapeStatusPending ||
			!reflect.DeepEqual(item.Maker, &Entity{Name: "Legacy studio"}) {
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
		for _, removed := range []string{"file_count", "size", "watched"} {
			if _, exists := fields[removed]; exists {
				t.Errorf("card still exposes unused field %s", removed)
			}
		}
		for _, required := range []string{"scrape_status"} {
			if _, exists := fields[required]; !exists {
				t.Errorf("card omits badge field %s", required)
			}
		}
	}
	first, err := lib.Movies(ctx, 1, 1)
	if err != nil || len(first.Movies) != 1 || !first.HasMore {
		t.Fatalf("first page: %#v, %v", first, err)
	}
	second, err := lib.Movies(ctx, 2, 1)
	if err != nil || len(second.Movies) != 1 || second.HasMore || first.Movies[0].ID == second.Movies[0].ID {
		t.Fatalf("second page: %#v, %v", second, err)
	}
	if beyond, err := lib.Movies(ctx, 3, 1); err != nil || beyond.Movies == nil || len(beyond.Movies) != 0 || beyond.HasMore {
		t.Fatalf("out-of-range page: %#v, %v", beyond, err)
	}
}

func TestLibraryPagesContainTwentyDistinctMoviesAndTheRemainder(t *testing.T) {
	lib, _, _ := libraryFixture(t)
	ctx := t.Context()
	created := time.Date(2026, time.September, 12, 0, 0, 0, 0, time.UTC)
	for index := range 21 {
		film := lib.database.Movie.Create().SetCode(fmt.Sprintf("PAGE-%03d", index)).SetCreatedAt(created).SaveX(ctx)
		for part := range 2 {
			lib.database.File.Create().SetFileID(fmt.Sprintf("%d-%d", index, part)).SetName("video.mp4").SetSize(1024).
				SetAccountID("100").SetRootID("10").SetMovieID(film.ID).ExecX(ctx)
		}
	}
	seen := make(map[int]bool)
	previousID := 0
	for pageNumber, count := range []int{20, 1, 0} {
		page, err := lib.Movies(ctx, pageNumber+1, 20)
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
	lib, _, _ := libraryFixture(t)
	for _, unmatched := range []bool{false, true} {
		if unmatched {
			lib.database.File.Create().SetFileID("unmatched").SetName("recording.mp4").SetSize(1024).
				SetAccountID("100").SetRootID("10").ExecX(t.Context())
		}
		page, err := lib.Movies(t.Context(), 1, 24)
		if err != nil || page.Total != 0 || page.Movies == nil || len(page.Movies) != 0 || page.HasMore {
			t.Fatalf("empty/unmatched source: %#v, %v", page, err)
		}
	}
}

func TestLibraryMatchingMovies(t *testing.T) {
	lib, _, payload := libraryFixture(t)
	ctx := t.Context()

	film1 := lib.database.Movie.Create().SetCode("ABP-001").SetJavdbID("javdb-1").SaveX(ctx)
	film2 := lib.database.Movie.Create().SetCode("ABP-002").SaveX(ctx)
	filmOther := lib.database.Movie.Create().SetCode("ABP-003").SetJavdbID("javdb-3").SaveX(ctx)

	// Files in mounted source
	lib.database.File.Create().SetFileID("f1").SetName("ABP-001.mp4").SetSize(1024).
		SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).SetMovie(film1).ExecX(ctx)
	lib.database.File.Create().SetFileID("f2").SetName("ABP-002.mp4").SetSize(1024).
		SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).SetMovie(film2).ExecX(ctx)
	// File in different source
	lib.database.File.Create().SetFileID("f3").SetName("ABP-003.mp4").SetSize(1024).
		SetAccountID("different-account").SetRootID("different-dir").SetMovie(filmOther).ExecX(ctx)

	matches, err := lib.MatchingMovies(ctx, []string{"javdb-1", "javdb-3"}, []string{"ABP-002", "ABP-999"})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d: %+v", len(matches), matches)
	}

	found1, found2 := false, false
	for _, m := range matches {
		if m.ID == film1.ID && m.Code == "ABP-001" && m.JavDBID != nil && *m.JavDBID == "javdb-1" {
			found1 = true
		}
		if m.ID == film2.ID && m.Code == "ABP-002" && m.JavDBID == nil {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Fatalf("expected matches for film1 and film2, got %+v", matches)
	}
}
