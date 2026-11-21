package library

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
)

func TestLibraryListingAndMatchingShareLocalAndMountedScope(t *testing.T) {
	lib, _, payload := libraryFixture(t)
	ctx := t.Context()
	var codes []string
	var visible []int
	for i, source := range []struct{ account, root string }{
		{domain.LocalAccountID, "local"},
		{domain.LocalAccountID, "another-local-root"},
		{payload.Source.AccountID, payload.Source.Directory.ID},
		{payload.Source.AccountID, "other-root"},
		{"other-account", payload.Source.Directory.ID},
	} {
		code := fmt.Sprintf("ABP-%03d", i+1)
		film := lib.database.Movie.Create().SetCode(code).SaveX(ctx)
		lib.database.File.Create().SetFileID(fmt.Sprint(i)).SetName(code + ".mp4").SetSize(1024).
			SetAccountID(source.account).SetRootID(source.root).SetMovieID(film.ID).ExecX(ctx)
		codes = append(codes, code)
		if i < 3 {
			visible = append(visible, film.ID)
		}
	}
	for _, mounted := range []bool{true, false} {
		if !mounted {
			if err := lib.drive.ClearDirectoryForAccount(ctx, "100"); err != nil {
				t.Fatal(err)
			}
			visible = visible[:2]
		}
		page, err := lib.Movies(ctx, 1, 20)
		if err != nil || page.Total != len(visible) || (page.Source != nil) != mounted {
			t.Fatalf("mounted=%t page=%+v error=%v", mounted, page, err)
		}
		matches, err := lib.MatchingMovies(ctx, codes)
		if err != nil || len(matches) != len(visible) || len(page.Movies) != len(visible) {
			t.Fatalf("mounted=%t page=%+v matches=%+v error=%v", mounted, page, matches, err)
		}
		for _, id := range visible {
			inPage, inMatches := false, false
			for _, film := range page.Movies {
				inPage = inPage || film.ID == id
			}
			for _, film := range matches {
				inMatches = inMatches || film.ID == id
			}
			if !inPage || !inMatches {
				t.Fatalf("mounted=%t missing movie %d in page or matches", mounted, id)
			}
		}
	}
}

func TestLibraryPageLoadsCardMetadataWithScopedCounts(t *testing.T) {
	lib, _, payload := libraryFixture(t)
	ctx := t.Context()
	database := lib.database
	tagB := database.Tag.Create().SetProvider("javdb").SetSourceID("tag-b").SetName("B tag").SetCategoryID("category").SaveX(ctx)
	tagA := database.Tag.Create().SetProvider("javdb").SetSourceID("tag-a").SetName("A tag").SetCategoryID("category").SaveX(ctx)
	actorB := database.Actor.Create().SetProvider("javdb").SetSourceID("actor-b").SetName("B actor").SaveX(ctx)
	actorA := database.Actor.Create().SetProvider("javdb").SetSourceID("actor-a").SetName("A actor").SaveX(ctx)
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
	// One file query counts the scoped movies, one flags which of the page's
	// movies have files in a 115 source for the source badge, and one reads the
	// page's file names for the Chinese-subtitle badge; a single subtitle query
	// covers sidecar subtitle names. All are batched per page, so the count is
	// constant regardless of the number of cards. Mounts and scope come from the
	// in-memory account registry, so the page issues no settings read.
	wantQueries := map[string]int{"*ent.FileQuery": 3, "*ent.SubtitleQuery": 1, "*ent.WatchHistoryQuery": 1, "*ent.MovieQuery": 1, "*ent.TagQuery": 1, "*ent.ActorQuery": 1}
	if !reflect.DeepEqual(queries, wantQueries) {
		t.Fatalf("library cards made redundant queries: %#v", queries)
	}
	for _, item := range page.Movies {
		if item.ID == film.ID {
			if item.Title != film.Title || item.Cover == nil || *item.Cover != "/cover" ||
				item.ScrapeStatus != movie.ScrapeStatusDone ||
				!reflect.DeepEqual(item.Tags, []Tag{{ID: tagA.ID, Provider: "javdb", SourceID: tagA.SourceID, Name: tagA.Name}, {ID: tagB.ID, Provider: "javdb", SourceID: tagB.SourceID, Name: tagB.Name}}) {
				t.Fatalf("card lost or reordered catalogue data: %#v", item)
			}
			if item.ReleaseDate != "2026-09-12" || item.Duration != 125 || item.Rating != 9 || item.Fanart != "/api/library/artwork/fanart" ||
				!reflect.DeepEqual(item.Maker, &Entity{ID: "maker-id", Name: "Studio"}) ||
				!reflect.DeepEqual(item.Series, &Entity{ID: "series-id", Name: "Series"}) ||
				!reflect.DeepEqual(item.Director, &Entity{ID: "director-id", Name: "Director"}) ||
				!reflect.DeepEqual(item.Actors, []Entity{{Provider: "javdb", ID: actorA.SourceID, Name: actorA.Name}, {Provider: "javdb", ID: actorB.SourceID, Name: actorB.Name}}) {
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

func TestLibraryOrdersBy115UploadTimeAndFlagsCloudSource(t *testing.T) {
	lib, _, payload := libraryFixture(t)
	ctx := t.Context()
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	create := func(code, account, root string, uploadTime int64, created time.Time) int {
		film := lib.database.Movie.Create().SetCode(code).SetCreatedAt(created).SaveX(ctx)
		lib.database.File.Create().SetFileID(code).SetName(code + ".mp4").SetSize(1).
			SetAccountID(account).SetRootID(root).SetUploadTime(uploadTime).SetMovieID(film.ID).SetCreatedAt(created).ExecX(ctx)
		return film.ID
	}
	// upload_time is the 115 upload timestamp, so it must drive the order even
	// when the local scan created the movie row later.
	lowUpload := create("LOW-UPT", payload.Source.AccountID, payload.Source.Directory.ID, 100, base.Add(10*time.Hour))
	highUpload := create("HIGH-UPT", payload.Source.AccountID, payload.Source.Directory.ID, 900, base)
	// Local titles have no 115 file and rank after every 115 title.
	local := create("LOC-001", domain.LocalAccountID, "local", 999999, base.Add(20*time.Hour))
	// A 115-account file outside the mounted root stays local: it must not be
	// badged as 115 nor take a 115 upload time.
	stray := create("STRAY-001", domain.LocalAccountID, "local", 1, base.Add(6*time.Hour))
	lib.database.File.Create().SetFileID("stray-cloud").SetName("stray.mp4").SetSize(1).
		SetAccountID(payload.Source.AccountID).SetRootID("other-root").SetUploadTime(9999999).SetMovieID(stray).ExecX(ctx)

	page, err := lib.Movies(ctx, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 4 || len(page.Movies) != 4 {
		t.Fatalf("unexpected page: %+v", page)
	}
	wantOrder := []int{highUpload, lowUpload, local, stray}
	for i, movie := range page.Movies {
		if movie.ID != wantOrder[i] {
			t.Fatalf("order[%d] = %d, want %d (%+v)", i, movie.ID, wantOrder[i], page.Movies)
		}
	}
	for _, movie := range page.Movies {
		wantSource := ""
		if movie.ID == highUpload || movie.ID == lowUpload {
			wantSource = "115"
		}
		if movie.Source != wantSource {
			t.Fatalf("movie %d source = %q, want %q", movie.ID, movie.Source, wantSource)
		}
	}
}

func TestLibraryBrowseFiltersSortAndOptions(t *testing.T) {
	lib, _, payload := libraryFixture(t)
	ctx := t.Context()
	db := lib.database
	cloudAccount, cloudRoot := payload.Source.AccountID, payload.Source.Directory.ID

	tagA := db.Tag.Create().SetProvider("javdb").SetSourceID("tag-a").SetName("A tag").SaveX(ctx)
	tagB := db.Tag.Create().SetProvider("javdb").SetSourceID("tag-b").SetName("B tag").SaveX(ctx)
	tagADup := db.Tag.Create().SetProvider("fanza").SetSourceID("tag-a-dup").SetName("A tag").SaveX(ctx)
	actorX := db.Actor.Create().SetProvider("javdb").SetSourceID("actor-x").SetName("X actor").SaveX(ctx)

	createMovie := func(code, title string, rating float64, release, makerID, makerName, seriesName string, local bool, tags ...*ent.Tag) int {
		builder := db.Movie.Create().SetCode(code).SetTitle(title).SetRating(rating).
			SetMakerID(makerID).SetMakerName(makerName).SetSeriesName(seriesName)
		if release != "" {
			releaseDate, err := time.Parse(time.DateOnly, release)
			if err != nil {
				t.Fatal(err)
			}
			builder.SetReleaseDate(releaseDate)
		}
		if len(tags) > 0 {
			builder.AddTags(tags...)
		}
		if code == "BRW-001" {
			builder.AddActors(actorX)
		}
		film := builder.SaveX(ctx)
		account, root := cloudAccount, cloudRoot
		if local {
			account, root = domain.LocalAccountID, "local"
		}
		db.File.Create().SetFileID(code).SetName(code + ".mp4").SetSize(1).
			SetAccountID(account).SetRootID(root).SetMovieID(film.ID).ExecX(ctx)
		return film.ID
	}

	m1 := createMovie("BRW-001", "Alpha", 4.5, "2026-01-10", "maker-1", "Studio One", "Series One", false, tagA)
	m2 := createMovie("BRW-002", "Beta", 3.6, "2026-03-05", "maker-2", "Studio Two", "", false, tagADup)
	m3 := createMovie("BRW-003", "Gamma", 5.0, "2026-02-01", "maker-3", "Studio Three", "Series Two", false, tagB)
	m4 := createMovie("LOC-001", "Delta", 0, "", "", "", "", true)

	ids := func(page Page) []int {
		result := make([]int, len(page.Movies))
		for i, item := range page.Movies {
			result[i] = item.ID
		}
		return result
	}

	all, err := lib.BrowseMovies(ctx, BrowseFilter{Page: 1, Limit: 20})
	if err != nil || all.Total != 4 || len(all.Movies) != 4 {
		t.Fatalf("browse all: %+v err=%v", all, err)
	}

	high, err := lib.BrowseMovies(ctx, BrowseFilter{Page: 1, Limit: 20, HighRated: true})
	if err != nil || high.Total != 2 {
		t.Fatalf("browse high: %+v err=%v", high, err)
	}
	for _, item := range high.Movies {
		if item.ID != m1 && item.ID != m3 {
			t.Fatalf("high rated filter leaked movie %d", item.ID)
		}
	}

	cloud, err := lib.BrowseMovies(ctx, BrowseFilter{Page: 1, Limit: 20, Source: "115"})
	if err != nil || cloud.Total != 3 {
		t.Fatalf("source=115: %+v err=%v", cloud, err)
	}
	local, err := lib.BrowseMovies(ctx, BrowseFilter{Page: 1, Limit: 20, Source: "local"})
	if err != nil || local.Total != 1 || local.Movies[0].ID != m4 {
		t.Fatalf("source=local: %+v err=%v", local, err)
	}

	byTag, err := lib.BrowseMovies(ctx, BrowseFilter{Page: 1, Limit: 20, TagNames: []string{"A tag"}})
	if err != nil || byTag.Total != 2 {
		t.Fatalf("tag filter: %+v err=%v", byTag, err)
	}
	if !containsID(ids(byTag), m1) || !containsID(ids(byTag), m2) {
		t.Fatalf("tag filter must match same-named tags across sources: %v", ids(byTag))
	}
	byActor, err := lib.BrowseMovies(ctx, BrowseFilter{Page: 1, Limit: 20, ActorNames: []string{"X actor"}})
	if err != nil || byActor.Total != 1 || byActor.Movies[0].ID != m1 {
		t.Fatalf("actor filter: %+v err=%v", byActor, err)
	}
	byMaker, err := lib.BrowseMovies(ctx, BrowseFilter{Page: 1, Limit: 20, MakerNames: []string{"Studio Two"}})
	if err != nil || byMaker.Total != 1 || byMaker.Movies[0].ID != m2 {
		t.Fatalf("maker filter: %+v err=%v", byMaker, err)
	}
	bySeries, err := lib.BrowseMovies(ctx, BrowseFilter{Page: 1, Limit: 20, SeriesNames: []string{"Series One"}})
	if err != nil || bySeries.Total != 1 || bySeries.Movies[0].ID != m1 {
		t.Fatalf("series filter: %+v err=%v", bySeries, err)
	}
	keyword, err := lib.BrowseMovies(ctx, BrowseFilter{Page: 1, Limit: 20, Query: "beta"})
	if err != nil || keyword.Total != 1 || keyword.Movies[0].ID != m2 {
		t.Fatalf("keyword filter: %+v err=%v", keyword, err)
	}
	byActorName, err := lib.BrowseMovies(ctx, BrowseFilter{Page: 1, Limit: 20, Query: "x actor"})
	if err != nil || byActorName.Total != 1 || byActorName.Movies[0].ID != m1 {
		t.Fatalf("keyword by actor name: %+v err=%v", byActorName, err)
	}

	rated, err := lib.BrowseMovies(ctx, BrowseFilter{Page: 1, Limit: 20, Sort: "rating"})
	if err != nil || len(rated.Movies) != 4 || rated.Movies[0].ID != m3 || rated.Movies[1].ID != m1 {
		t.Fatalf("rating sort: %v err=%v", ids(rated), err)
	}
	released, err := lib.BrowseMovies(ctx, BrowseFilter{Page: 1, Limit: 20, Sort: "release"})
	if err != nil || !reflect.DeepEqual(ids(released), []int{m2, m3, m1, m4}) {
		t.Fatalf("release sort: %v err=%v", ids(released), err)
	}

	second, err := lib.BrowseMovies(ctx, BrowseFilter{Page: 2, Limit: 2, Sort: "rating"})
	if err != nil || second.Total != 4 || len(second.Movies) != 2 || second.HasMore {
		t.Fatalf("page 2: %+v err=%v", second, err)
	}
	clamped, err := lib.BrowseMovies(ctx, BrowseFilter{Page: 9, Limit: 2, Sort: "rating"})
	if err != nil || clamped.Page != 2 || len(clamped.Movies) != 2 || clamped.HasMore {
		t.Fatalf("clamped page: %+v err=%v", clamped, err)
	}
	bogus, err := lib.BrowseMovies(ctx, BrowseFilter{Page: 1, Limit: 20, Sort: "nope", Source: "nope"})
	if err != nil || bogus.Total != 4 {
		t.Fatalf("bogus filters fallback: %+v err=%v", bogus, err)
	}

	canvas := func(options []FilterOption) []string {
		names := make([]string, len(options))
		for i, option := range options {
			names[i] = option.Name
		}
		return names
	}
	filterOptions, err := lib.FilterOptions(ctx)
	if err != nil {
		t.Fatalf("filter options: %v", err)
	}
	if !reflect.DeepEqual(canvas(filterOptions.Tags), []string{"A tag", "B tag"}) {
		t.Fatalf("tag options: %v", canvas(filterOptions.Tags))
	}
	if !reflect.DeepEqual(canvas(filterOptions.Actors), []string{"X actor"}) {
		t.Fatalf("actor options: %v", canvas(filterOptions.Actors))
	}
	if !reflect.DeepEqual(canvas(filterOptions.Makers), []string{"Studio One", "Studio Three", "Studio Two"}) {
		t.Fatalf("maker options: %v", canvas(filterOptions.Makers))
	}
	if !reflect.DeepEqual(canvas(filterOptions.Series), []string{"Series One", "Series Two"}) {
		t.Fatalf("series options: %v", canvas(filterOptions.Series))
	}
}

func TestLibraryTopRatedSortsAndTruncates(t *testing.T) {
	lib, _, payload := libraryFixture(t)
	ctx := t.Context()
	db := lib.database

	create := func(code string, rating float64) int {
		film := db.Movie.Create().SetCode(code).SetRating(rating).SaveX(ctx)
		db.File.Create().SetFileID(code).SetName(code + ".mp4").SetSize(1).
			SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).
			SetMovieID(film.ID).ExecX(ctx)
		return film.ID
	}
	m1 := create("TOP-001", 4.5) // 9.0
	m2 := create("TOP-002", 5.0) // 10.0
	m3 := create("TOP-003", 3.6) // 7.2
	create("TOP-004", 0)         // rated 0 -> excluded
	m5 := create("TOP-005", 4.5) // 9.0, newer id wins the tie
	// Rated but with no file in scope -> excluded.
	db.Movie.Create().SetCode("TOP-006").SetRating(5.0).SaveX(ctx)

	top, err := lib.TopRated(ctx, 10)
	if err != nil {
		t.Fatalf("top rated: %v", err)
	}
	if got, want := topRatedIDs(top), []int{m2, m5, m1, m3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("top rated order = %v, want %v", got, want)
	}
	wantRatings := map[int]float64{
		m1: domain.NormalizeRating10(4.5, 0),
		m2: domain.NormalizeRating10(5.0, 0),
		m3: domain.NormalizeRating10(3.6, 0),
		m5: domain.NormalizeRating10(4.5, 0),
	}
	for _, item := range top {
		if want := wantRatings[item.ID]; item.Rating != want {
			t.Fatalf("movie %d rating = %v, want %v", item.ID, item.Rating, want)
		}
	}

	limited, err := lib.TopRated(ctx, 2)
	if err != nil {
		t.Fatalf("top rated limited: %v", err)
	}
	if got, want := topRatedIDs(limited), []int{m2, m5}; !reflect.DeepEqual(got, want) {
		t.Fatalf("top rated limit 2 = %v, want %v", got, want)
	}
}

func topRatedIDs(movies []Movie) []int {
	out := make([]int, len(movies))
	for i, item := range movies {
		out[i] = item.ID
	}
	return out
}

func containsID(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestLibraryMatchingMovies(t *testing.T) {
	lib, _, payload := libraryFixture(t)
	ctx := t.Context()

	film1 := lib.database.Movie.Create().SetCode("ABP-001").SaveX(ctx)
	film2 := lib.database.Movie.Create().SetCode("ABP-002").SaveX(ctx)
	filmOther := lib.database.Movie.Create().SetCode("ABP-003").SaveX(ctx)

	// Files in mounted source
	lib.database.File.Create().SetFileID("f1").SetName("ABP-001.mp4").SetSize(1024).
		SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).SetMovie(film1).ExecX(ctx)
	lib.database.File.Create().SetFileID("f2").SetName("ABP-002.mp4").SetSize(1024).
		SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).SetMovie(film2).ExecX(ctx)
	// File in different source
	lib.database.File.Create().SetFileID("f3").SetName("ABP-003.mp4").SetSize(1024).
		SetAccountID("different-account").SetRootID("different-dir").SetMovie(filmOther).ExecX(ctx)

	matches, err := lib.MatchingMovies(ctx, []string{"ABP-001", "ABP-002", "ABP-999"})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d: %+v", len(matches), matches)
	}

	found1, found2 := false, false
	for _, m := range matches {
		if m.ID == film1.ID && m.Code == "ABP-001" {
			found1 = true
		}
		if m.ID == film2.ID && m.Code == "ABP-002" {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Fatalf("expected matches for film1 and film2, got %+v", matches)
	}
}
