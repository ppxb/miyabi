package image

import (
	"bytes"
	stdimage "image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/disintegration/imaging"
	"github.com/ppxb/miyabi/internal/domain"
)

func TestPosterWindowKeepsFacesAndBounds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		bounds stdimage.Rectangle
		faces  []posterFace
		keep   []stdimage.Rectangle
		want   stdimage.Rectangle
	}{
		{name: "no face retains jacket front", bounds: stdimage.Rect(0, 0, 900, 600), want: stdimage.Rect(500, 0, 900, 600)},
		{name: "tall portrait without faces", bounds: stdimage.Rect(0, 0, 400, 900), want: stdimage.Rect(0, 0, 400, 600)},
		{name: "left edge", bounds: stdimage.Rect(0, 0, 900, 600), faces: []posterFace{{stdimage.Rect(0, 10, 160, 240), 10}}, keep: []stdimage.Rectangle{stdimage.Rect(0, 10, 160, 240)}},
		{name: "right edge", bounds: stdimage.Rect(0, 0, 900, 600), faces: []posterFace{{stdimage.Rect(750, 10, 900, 240), 10}}, keep: []stdimage.Rectangle{stdimage.Rect(750, 10, 900, 240)}},
		{name: "nearby group", bounds: stdimage.Rect(0, 0, 900, 600), faces: []posterFace{{stdimage.Rect(220, 20, 330, 160), 10}, {stdimage.Rect(420, 30, 530, 170), 8}}, keep: []stdimage.Rectangle{stdimage.Rect(220, 20, 330, 160), stdimage.Rect(420, 30, 530, 170)}},
		{name: "distant group follows primary", bounds: stdimage.Rect(0, 0, 1200, 600), faces: []posterFace{{stdimage.Rect(70, 20, 240, 220), 10}, {stdimage.Rect(970, 20, 1140, 220), 5}}, keep: []stdimage.Rectangle{stdimage.Rect(70, 20, 240, 220)}},
		{name: "portrait face near bottom", bounds: stdimage.Rect(0, 0, 400, 1200), faces: []posterFace{{stdimage.Rect(90, 850, 290, 1090), 10}}, keep: []stdimage.Rectangle{stdimage.Rect(90, 850, 290, 1090)}},
		{name: "translated bounds", bounds: stdimage.Rect(100, 200, 1000, 800), faces: []posterFace{{stdimage.Rect(110, 210, 270, 440), 10}}, keep: []stdimage.Rectangle{stdimage.Rect(110, 210, 270, 440)}},
		{name: "one pixel", bounds: stdimage.Rect(0, 0, 1, 1), want: stdimage.Rect(0, 0, 1, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := posterWindow(tc.bounds, tc.faces, "jacket")
			if got.Empty() || !got.In(tc.bounds) {
				t.Fatalf("crop %v escaped source %v", got, tc.bounds)
			}
			if tc.want != (stdimage.Rectangle{}) && got != tc.want {
				t.Fatalf("crop = %v, want %v", got, tc.want)
			}
			for _, face := range tc.keep {
				if !face.In(got) {
					t.Errorf("crop %v cuts face %v", got, face)
				}
			}
		})
	}
}

func TestPosterLayoutBoundsAndDefaults(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		bounds, region, crop stdimage.Rectangle
		layout               domain.CoverLayout
	}{
		{"jacket", stdimage.Rect(0, 0, 900, 600), stdimage.Rect(405, 0, 900, 600), stdimage.Rect(500, 0, 900, 600), domain.CoverJacket},
		{"single", stdimage.Rect(0, 0, 900, 600), stdimage.Rect(0, 0, 900, 600), stdimage.Rect(250, 0, 650, 600), domain.CoverSingle},
		{"portrait", stdimage.Rect(0, 0, 600, 900), stdimage.Rect(0, 0, 600, 900), stdimage.Rect(0, 0, 600, 900), domain.CoverJacket},
		{"translated jacket", stdimage.Rect(100, 200, 1000, 800), stdimage.Rect(505, 200, 1000, 800), stdimage.Rect(600, 200, 1000, 800), domain.CoverJacket},
	} {
		t.Run(tc.name, func(t *testing.T) {
			region := posterRegion(tc.bounds, tc.layout)
			if region != tc.region {
				t.Fatalf("detection region=%v want=%v", region, tc.region)
			}
			if crop := posterWindow(region, nil, tc.layout); crop != tc.crop {
				t.Fatalf("default crop=%v want=%v", crop, tc.crop)
			}
		})
	}
}

func TestJacketCropExcludesLargerBackCoverFace(t *testing.T) {
	face := posterFixture(t)
	cover := imaging.New(1200, 800, color.NRGBA{B: 200, A: 255})
	cover = imaging.Paste(cover, imaging.Resize(face, 440, 0, imaging.Lanczos), stdimage.Pt(15, 30))
	cover = imaging.Paste(cover, face, stdimage.Pt(900, 120))
	poster, err := cropPoster(cover, domain.CoverJacket)
	if err != nil {
		t.Fatal(err)
	}
	if poster.Bounds().Dx() != 533 || poster.Bounds().Dy() != 800 {
		t.Fatalf("lost original pixels: %v", poster.Bounds())
	}
	// The crop can start anywhere in [540,667]; this whole rectangle is
	// inside the front face for every legal window, never the back face.
	faces, err := detectPosterFaces(poster)
	if err != nil || len(faces) == 0 {
		t.Fatalf("front face lost: %v %v", faces, err)
	}
	for _, detected := range faces {
		if detected.bounds.Dx() > 350 {
			t.Fatalf("back cover face influenced output: %+v", faces)
		}
	}
}

func posterFixture(t testing.TB) stdimage.Image {
	t.Helper()
	path := os.Getenv("MIYABI_TEST_FACE_IMAGE")
	if path == "" {
		t.Skip("set MIYABI_TEST_FACE_IMAGE to a real portrait outside the repository")
	}
	source, err := imaging.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return imaging.Resize(source, 160, 0, imaging.Lanczos)
}

func TestPosterDetectsFacesAcrossCoverLayouts(t *testing.T) {
	face := posterFixture(t)
	for _, tc := range []struct {
		name  string
		image stdimage.Image
		x, y  int
	}{
		{"left", face, 15, 70},
		{"center", face, 380, 70},
		{"right", face, 725, 70},
		{"sideways left", imaging.Rotate90(face), 15, 70},
		{"sideways right", imaging.Rotate270(face), 650, 70},
		{"tilted", imaging.Rotate(face, 35, color.White), 250, 70},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cover := imaging.Paste(imaging.New(900, 600, color.White), tc.image, stdimage.Pt(tc.x, tc.y))
			faces, err := detectPosterFaces(cover)
			if err != nil || len(faces) == 0 {
				t.Fatalf("detect faces: %+v, %v", faces, err)
			}
			crop := posterWindow(cover.Bounds(), faces, "single")
			center := stdimage.Pt(tc.x+tc.image.Bounds().Dx()/2, tc.y+tc.image.Bounds().Dy()/2)
			// A central region around the known face must survive the crop;
			// checking only the detector's own rectangles would hide mapping errors.
			important := stdimage.Rect(center.X-50, center.Y-50, center.X+50, center.Y+50)
			if !important.In(crop) {
				t.Fatalf("crop %v missed the known face at %v; detections=%+v", crop, important, faces)
			}
		})
	}
}

func TestPosterPreservesResolutionAndExistingArtwork(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Already composed 2:3 artwork needs neither detection nor downscaling.
	cover := imaging.New(1200, 1800, color.White)
	var body bytes.Buffer
	if err := png.Encode(&body, cover); err != nil {
		t.Fatal(err)
	}
	artwork, err := cache.FromCover(body.Bytes(), "single")
	if err != nil {
		t.Fatal(err)
	}
	poster, err := cache.ReadURL(artwork.Poster)
	if err != nil {
		t.Fatal(err)
	}
	config, _, err := stdimage.DecodeConfig(bytes.NewReader(poster))
	if err != nil || config.Width != 1200 || config.Height != 1800 {
		t.Fatalf("poster was downscaled: %+v, %v", config, err)
	}
	recropped, err := cache.RecropPoster(artwork, "single")
	if err != nil || recropped.Fanart != artwork.Fanart || recropped.Thumbnail != artwork.Thumbnail || recropped.Poster != artwork.Poster {
		t.Fatalf("recrop changed unmodified artwork: %+v, %v", recropped, err)
	}
	// Explicitly supplied NFO posters keep their authored composition.
	restored, err := cache.Restore(body.Bytes(), body.Bytes())
	if err != nil || restored.Poster != artwork.Poster {
		t.Fatalf("restore changed an authored poster: %+v, %v", restored, err)
	}
}

func BenchmarkPosterFaceDetection(b *testing.B) {
	face := posterFixture(b)
	cover := imaging.Paste(imaging.New(1800, 1200, color.White), imaging.Resize(face, 480, 0, imaging.Lanczos), stdimage.Pt(150, 180))
	b.ResetTimer()
	for b.Loop() {
		if _, err := cropPoster(cover, "single"); err != nil {
			b.Fatal(err)
		}
	}
}
