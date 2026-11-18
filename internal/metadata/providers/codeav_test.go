package providers

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCodeAVSlugs(t *testing.T) {
	if got, want := codeAVSlugs("MEYD-589CH"), []string{"meyd-589ch", "meyd-589"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("slugs = %v, want %v", got, want)
	}
}

func TestParseCodeAVFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/codeav_ssis-001.html")
	if err != nil {
		t.Fatal(err)
	}
	result, err := parseCodeAVHTML(body, "SSIS-001")
	if err != nil {
		t.Fatal(err)
	}
	movie := result.Detail.Movie
	if movie.Code != "SSIS-001" || movie.ReleaseDate != "2021-02-18" || movie.Duration != 147 {
		t.Fatalf("scalars = %+v", movie)
	}
	if !strings.Contains(movie.Title, "一ヶ月間") || !strings.Contains(movie.OriginTitle, "After a Month") {
		t.Fatalf("titles = %q / %q", movie.Title, movie.OriginTitle)
	}
	if movie.Maker == nil || movie.Maker.Name != "エスワン ナンバーワンスタイル" {
		t.Fatalf("maker = %+v", movie.Maker)
	}
	if movie.Director == nil || movie.Director.Name != "苺原" {
		t.Fatalf("director = %+v", movie.Director)
	}
	if len(movie.Actors) == 0 || len(movie.Tags) < 8 {
		t.Fatalf("entities = actors %d tags %d", len(movie.Actors), len(movie.Tags))
	}
	if !strings.HasPrefix(movie.Cover, "https://awsimgsrc.dmm.co.jp/") {
		t.Fatalf("cover = %q", movie.Cover)
	}
	if movie.Rating <= 0 || len(result.Images) < 5 {
		t.Fatalf("rating/images = %v / %d", movie.Rating, len(result.Images))
	}
}
