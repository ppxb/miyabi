package providers

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/metadata"
)

func TestR18ContentIDs(t *testing.T) {
	if got, want := r18ContentIDs("SSIS-001"), []string{"ssis00001", "ssis001"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("content ids = %v, want %v", got, want)
	}
}

func TestParseR18Fixture(t *testing.T) {
	body, err := os.ReadFile("testdata/r18_ssis00001.json")
	if err != nil {
		t.Fatal(err)
	}
	result, err := parseR18JSON(body, "SSIS-001", "ssis00001")
	if err != nil {
		t.Fatal(err)
	}
	movie := result.Detail.Movie
	if movie.Code != "SSIS-001" || movie.ReleaseDate != "2021-02-19" || movie.Duration != 147 {
		t.Fatalf("scalars = %+v", movie)
	}
	if !strings.Contains(movie.Title, "After Abstaining") {
		t.Fatalf("title = %q", movie.Title)
	}
	if !strings.HasPrefix(movie.Cover, "https://pics.r18.com/") {
		t.Fatalf("cover = %q", movie.Cover)
	}
	if len(movie.Actors) != 2 || movie.Actors[0].Name != "Tsukasa Aoi" {
		t.Fatalf("actors = %+v", movie.Actors)
	}
	if len(movie.Tags) != 5 {
		t.Fatalf("tags = %+v", movie.Tags)
	}
	if movie.Maker == nil || movie.Maker.Name != "S1 NO.1 STYLE" {
		t.Fatalf("maker = %+v", movie.Maker)
	}
	if movie.Series != nil {
		t.Fatalf("unexpected series = %+v", movie.Series)
	}
	if len(result.Images) != 11 || result.Images[0].Role != "cover" {
		t.Fatalf("images = %d", len(result.Images))
	}
}

func TestR18MismatchedDVDID(t *testing.T) {
	body, err := os.ReadFile("testdata/r18_ssis00001.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseR18JSON(body, "SSIS-002", "ssis00001"); err == nil {
		t.Fatal("expected not found for mismatched dvd id")
	}
}

func TestParseOnePondoFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/onepondo_050826_001.json")
	if err != nil {
		t.Fatal(err)
	}
	result, err := parseOnePondoJSON(body, "1PONDO-050826-001")
	if err != nil {
		t.Fatal(err)
	}
	movie := result.Detail.Movie
	if movie.Code != "1PONDO-050826-001" || movie.ReleaseDate != "2026-05-08" || movie.Duration != 61 {
		t.Fatalf("scalars = %+v", movie)
	}
	if len(movie.Actors) == 0 || len(movie.Tags) == 0 {
		t.Fatalf("movie entities = %+v", movie)
	}
	if movie.Series == nil || movie.Series.Name == "" {
		t.Fatalf("series = %+v", movie.Series)
	}
	if !strings.Contains(movie.Cover, "/moviepages/050826_001/") {
		t.Fatalf("cover = %q", movie.Cover)
	}
	if len(result.Images) != 1 || result.Images[0].Role != "cover" {
		t.Fatalf("images = %+v", result.Images)
	}
}

// TestParseOnePondoRejectsMismatchedResponse guards the shared identity checks:
// a wrong site, wrong movie or empty title must be treated as not found.
func TestParseOnePondoRejectsMismatchedResponse(t *testing.T) {
	for _, body := range []string{
		`{"MovieID":"050826_001","SiteID":2471,"Title":"x"}`,
		`{"MovieID":"999999_999","SiteID":2470,"Title":"x"}`,
		`{"MovieID":"050826_001","SiteID":2470}`,
	} {
		if _, err := parseOnePondoJSON([]byte(body), "1PONDO-050826-001"); !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("body %s accepted: %v", body, err)
		}
	}
}
