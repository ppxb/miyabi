package providers

import (
	"os"
	"strings"
	"testing"
)

func TestParseJavDatabaseFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/javdatabase_ssis-001.html")
	if err != nil {
		t.Fatal(err)
	}
	result, err := parseJavDatabaseHTML(body, "SSIS-001")
	if err != nil {
		t.Fatal(err)
	}
	movie := result.Detail.Movie
	if movie.Code != "SSIS-001" || movie.ReleaseDate != "2021-02-18" || movie.Duration != 147 {
		t.Fatalf("scalars = %+v", movie)
	}
	if !strings.Contains(movie.Title, "After Abstaining") {
		t.Fatalf("title = %q", movie.Title)
	}
	if movie.Maker == nil || movie.Maker.Name != "S1 NO.1 STYLE" {
		t.Fatalf("maker = %+v", movie.Maker)
	}
	if len(movie.Actors) != 2 || movie.Actors[0].Name != "Sayaka Otoshiro" {
		t.Fatalf("actors = %+v", movie.Actors)
	}
	if len(movie.Tags) < 6 {
		t.Fatalf("tags = %+v", movie.Tags)
	}
	if !strings.HasPrefix(movie.Cover, "https://pics.dmm.co.jp/") {
		t.Fatalf("cover = %q", movie.Cover)
	}
	if len(result.Images) != 1 || result.Images[0].Role != "cover" {
		t.Fatalf("images = %+v", result.Images)
	}
}

func TestParseJavDatabaseRejectsMismatchedDVDD(t *testing.T) {
	body, err := os.ReadFile("testdata/javdatabase_ssis-001.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseJavDatabaseHTML(body, "SSIS-002"); err == nil {
		t.Fatal("expected not found for mismatched dvd id")
	}
}
