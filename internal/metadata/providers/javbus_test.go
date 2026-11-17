package providers

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
)

type stubJavBus struct {
	body string
}

func (s stubJavBus) MoviePage(context.Context, string) (string, error) {
	return s.body, nil
}

func javbusFixture(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("../../javbus/testdata/detail_ssis-001.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestJavBusFetchParsesDetailFixture(t *testing.T) {
	provider := NewJavBus(stubJavBus{body: javbusFixture(t)}, nil)
	result, err := provider.Fetch(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if err != nil {
		t.Fatal(err)
	}
	movie := result.Detail.Movie
	if movie.Code != "SSIS-001" || movie.ReleaseDate != "2021-02-18" || movie.Duration != 150 {
		t.Fatalf("scalar fields = %+v", movie)
	}
	if movie.Cover != "https://www.javbus.com/pics/cover/83ie_b.jpg" {
		t.Fatalf("cover = %q", movie.Cover)
	}
	if len(movie.Actors) != 2 || movie.Actors[0].Name == "" {
		t.Fatalf("actors = %+v", movie.Actors)
	}
	if len(movie.Tags) == 0 {
		t.Fatalf("tags empty")
	}
	if movie.Maker == nil || movie.Maker.Name != "エスワン ナンバーワンスタイル" {
		t.Fatalf("maker = %+v", movie.Maker)
	}
	if movie.Series != nil {
		t.Fatalf("unexpected series = %+v", movie.Series)
	}
	if len(result.Images) == 0 || result.Images[0].Role != "cover" {
		t.Fatalf("images = %+v", result.Images)
	}
}

func TestJavBusFetchRejectsMismatchedCode(t *testing.T) {
	provider := NewJavBus(stubJavBus{body: javbusFixture(t)}, nil)
	if _, err := provider.Fetch(t.Context(), domain.MovieRef{Code: "SSIS-002"}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestJavBusFetchMissingPage(t *testing.T) {
	provider := NewJavBus(stubJavBus{body: ""}, nil)
	if _, err := provider.Fetch(t.Context(), domain.MovieRef{Code: "SSIS-001"}); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}
