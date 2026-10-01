package scrape

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/nfo"
)

type artworkSource map[string][]byte

func (artworkSource) Resolve(context.Context, domain.MovieRef) (domain.MovieMetadata, error) {
	panic("metadata should not be queried")
}
func (artworkSource) Fallback(context.Context, domain.MovieRef) (domain.MovieMetadata, error) {
	panic("fallback should not be queried")
}
func (s artworkSource) Image(_ context.Context, candidate domain.ImageCandidate) (domain.Media, error) {
	return domain.Media{Body: s[candidate.URL]}, nil
}

func TestArtworkSelectionRejectsCorruptLargerImageAndUsesEffectivePixels(t *testing.T) {
	encode := func(w, h int) []byte {
		t.Helper()
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
			t.Fatal(err)
		}
		return buffer.Bytes()
	}
	good := encode(120, 180)
	corrupt := encode(1200, 1800)[:33] // Valid dimensions, truncated pixels.
	service := &Service{metadata: artworkSource{"bad": corrupt, "small": encode(24, 16), "wide": encode(1200, 60), "good": good}}
	var candidates []domain.ImageCandidate
	for _, url := range []string{"bad", "small", "wide", "good"} {
		candidates = append(candidates, domain.ImageCandidate{Provider: "fixture", URL: url, Role: "cover"})
	}
	body, selected, err := service.selectCover(t.Context(), candidates)
	if err != nil || selected.URL != "good" || !bytes.Equal(body, good) {
		t.Fatalf("selection: %+v %v", selected, err)
	}
	// A fallback cannot replace an available primary image, even if listed first.
	candidates = []domain.ImageCandidate{{Provider: "javdb", URL: "good", Role: "cover"}, {Provider: "fanza", URL: "small", Role: "cover"}}
	_, selected, err = service.selectCover(t.Context(), candidates)
	if err != nil || selected.Provider != "fanza" {
		t.Fatalf("fallback overrode primary: %+v %v", selected, err)
	}
	candidates[1].URL = "bad"
	_, selected, err = service.selectCover(t.Context(), candidates)
	if err != nil || selected.Provider != "javdb" {
		t.Fatalf("broken primary prevented fallback: %+v %v", selected, err)
	}
}

func TestAuthoredPosterDoesNotRunCropOnPolicyChange(t *testing.T) {
	service := &Service{}
	input := Payload{Document: nfo.Movie{SelectedImage: domain.ImageCandidate{Role: "poster"}}, Artwork: &mediaimage.Artwork{Poster: "original"}}
	if err := service.prepareArtwork(t.Context(), 0, &input); err != nil {
		t.Fatal(err)
	}
	if input.Artwork.Poster != "original" || input.PosterVersion != mediaimage.PosterVersion {
		t.Fatalf("authored poster changed: %+v", input)
	}
}
