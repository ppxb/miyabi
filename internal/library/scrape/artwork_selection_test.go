package scrape

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/tasks"
)

type artworkSource map[string][]byte

func (artworkSource) Resolve(context.Context, domain.MovieRef) (domain.MovieMetadata, error) {
	panic("metadata should not be queried")
}

type upgradeSource struct {
	artworkSource
	result domain.MovieMetadata
	err    error
	calls  int
}

func (s *upgradeSource) Fallback(context.Context, domain.MovieRef) (domain.MovieMetadata, error) {
	s.calls++
	return s.result, s.err
}

func TestLowResolutionUpgradeKeepsUsableArtworkAndCheckpointsLayout(t *testing.T) {
	encode := func(width, height int) []byte {
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
			t.Fatal(err)
		}
		return buffer.Bytes()
	}
	for _, tc := range []struct {
		name, primary, upgrade, selected string
		failure                          error
		calls                            int
	}{
		{"sharper fallback", "small", "large", "javdb", nil, 1},
		{"worse fallback", "medium", "small", "fanza", nil, 1},
		{"equal fallback", "medium", "medium", "fanza", nil, 1},
		{"unavailable fallback", "small", "large", "fanza", errors.New("unavailable"), 1},
		{"corrupt fallback", "small", "corrupt", "fanza", nil, 1},
		{"unusable primary", "corrupt", "large", "javdb", nil, 1},
		{"adequate official", "large", "large", "fanza", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := database.Open(t.Context(), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			cache, err := mediaimage.NewCache(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			source := &upgradeSource{artworkSource: artworkSource{"small": encode(60, 40), "medium": encode(300, 200), "large": encode(1200, 1800), "corrupt": []byte("invalid image")}, err: tc.failure,
				result: domain.MovieMetadata{Detail: domain.MovieDetail{Movie: domain.Movie{Sources: []domain.SourceID{{Provider: "javdb", ID: "confirmed"}}}, Zone: domain.ZoneCensored},
					Images: []domain.ImageCandidate{{Provider: "javdb", URL: tc.upgrade, Role: "cover"}}}}
			service := &Service{db: store.Client, images: cache, metadata: source}
			job := store.Client.Task.Create().SetType("scrape").SaveX(t.Context())
			input := Payload{Document: nfo.Movie{Zone: domain.ZoneCensored, Images: []domain.ImageCandidate{{Provider: "fanza", URL: tc.primary, Role: "cover"}}}}
			if err := service.prepareArtwork(t.Context(), job.ID, &input); err != nil {
				t.Fatal(err)
			}
			if input.Document.SelectedImage.Provider != tc.selected || source.calls != tc.calls {
				t.Fatalf("selection=%+v calls=%d", input.Document.SelectedImage, source.calls)
			}
			selected := input.Document.SelectedImage
			poster, err := cache.ReadURL(input.Artwork.Poster)
			if err != nil {
				t.Fatal(err)
			}
			config, _, err := image.DecodeConfig(bytes.NewReader(poster))
			width, height := mediaimage.PosterSize(selected.Width, selected.Height, selected.Layout)
			if err != nil || config.Width != width || config.Height != height {
				t.Fatalf("poster resolution changed: %+v, want %dx%d: %v", config, width, height, err)
			}
			saved, err := tasks.DecodePayload[Payload](store.Client.Task.GetX(t.Context(), job.ID).Payload)
			if err != nil || saved.Document.SelectedImage.Layout != domain.CoverJacket || saved.PosterVersion != mediaimage.PosterVersion {
				t.Fatalf("checkpoint=%+v %v", saved, err)
			}
			if err := service.prepareArtwork(t.Context(), job.ID, &saved); err != nil || source.calls != tc.calls {
				t.Fatalf("retry repeated upgrade: calls=%d err=%v", source.calls, err)
			}
		})
	}
}

func TestAuthoredPosterSelectionAndOriginalDimensions(t *testing.T) {
	images := artworkSource{}
	for name, size := range map[string]image.Point{
		"poster": {X: 600, Y: 1000}, "short": {X: 600, Y: 700}, "narrow": {X: 500, Y: 1200},
		"large": {X: 1200, Y: 1800}, "small": {X: 60, Y: 90},
	} {
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, image.NewRGBA(image.Rectangle{Max: size})); err != nil {
			t.Fatal(err)
		}
		images[name] = buffer.Bytes()
	}
	service := &Service{metadata: images}
	for _, tc := range []struct{ poster, cover, provider, want string }{
		{"poster", "large", "fanza", "poster"},
		{"poster", "large", "javdb", "poster"},
		{"short", "large", "javdb", "large"},
		{"narrow", "large", "javdb", "large"},
		{"short", "small", "javdb", "short"},
	} {
		candidates := []domain.ImageCandidate{
			{Provider: "fanza", URL: tc.poster, Role: "poster"},
			{Provider: tc.provider, URL: tc.cover, Role: "cover"},
		}
		_, selected, err := service.selectCover(t.Context(), candidates, domain.ZoneCensored)
		if err != nil || selected.URL != tc.want {
			t.Fatalf("selection %+v: %+v, %v", tc, selected, err)
		}
	}
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cache, err := mediaimage.NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service.db, service.images = store.Client, cache
	job := store.Client.Task.Create().SetType("scrape").SaveX(t.Context())
	input := Payload{Document: nfo.Movie{Images: []domain.ImageCandidate{{Provider: "fanza", URL: "poster", Role: "poster"}}}}
	if err := service.prepareArtwork(t.Context(), job.ID, &input); err != nil {
		t.Fatal(err)
	}
	poster, err := cache.ReadURL(input.Artwork.Poster)
	if err != nil {
		t.Fatal(err)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(poster))
	if err != nil || config.Width != 600 || config.Height != 1000 {
		t.Fatalf("authored poster was cropped or resized: %+v, %v", config, err)
	}
}

func TestCoverLayoutUsesExplicitSourceBeforeCategory(t *testing.T) {
	for _, tc := range []struct {
		zone       domain.Zone
		hint, want domain.CoverLayout
	}{
		{domain.ZoneCensored, "", domain.CoverJacket},
		{domain.ZoneCensored, domain.CoverSingle, domain.CoverSingle},
		{domain.ZoneFC2, "", domain.CoverSingle},
		{domain.ZoneUncensored, "", domain.CoverSingle},
		{domain.ZoneWestern, "", domain.CoverSingle},
		{domain.ZoneUnknown, "", domain.CoverSingle},
		{domain.ZoneUnknown, domain.CoverJacket, domain.CoverJacket},
	} {
		if got := coverLayout(domain.ImageCandidate{Layout: tc.hint}, tc.zone); got != tc.want {
			t.Errorf("layout=%s want=%s", got, tc.want)
		}
	}
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
	service := &Service{metadata: artworkSource{"bad": corrupt, "small": encode(24, 16), "wide": encode(1200, 60), "good": good,
		"official": encode(600, 900), "large": encode(1200, 1800)}}
	var candidates []domain.ImageCandidate
	for _, url := range []string{"bad", "small", "wide", "good"} {
		candidates = append(candidates, domain.ImageCandidate{Provider: "fixture", URL: url, Role: "cover"})
	}
	body, selected, err := service.selectCover(t.Context(), candidates, domain.ZoneUnknown)
	if err != nil || selected.URL != "good" || !bytes.Equal(body, good) {
		t.Fatalf("selection: %+v %v", selected, err)
	}
	// A sufficient primary image needs no extra fallback download.
	candidates = []domain.ImageCandidate{{Provider: "javdb", URL: "large", Role: "cover"}, {Provider: "fanza", URL: "official", Role: "cover"}}
	_, selected, err = service.selectCover(t.Context(), candidates, domain.ZoneUnknown)
	if err != nil || selected.Provider != "fanza" {
		t.Fatalf("fallback overrode primary: %+v %v", selected, err)
	}
	candidates[1].URL = "small"
	_, selected, err = service.selectCover(t.Context(), candidates, domain.ZoneUnknown)
	if err != nil || selected.Provider != "javdb" {
		t.Fatalf("low-resolution primary prevented sharper fallback: %+v %v", selected, err)
	}
	candidates[0].URL, candidates[1].URL = "bad", "small"
	_, selected, err = service.selectCover(t.Context(), candidates, domain.ZoneUnknown)
	if err != nil || selected.Provider != "fanza" {
		t.Fatalf("unavailable upgrade discarded usable original: %+v %v", selected, err)
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
