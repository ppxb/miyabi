package image

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"

	"github.com/disintegration/imaging"
)

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	source := imaging.New(width, height, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, source); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestThumbnailResizesWideImages(t *testing.T) {
	body, resized := Thumbnail(testPNG(t, 200, 100), 80)
	if !resized {
		t.Fatal("expected a resized image")
	}
	decoded, err := imaging.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.Bounds().Dx(); got != 80 {
		t.Fatalf("width = %d, want 80", got)
	}
	if got := decoded.Bounds().Dy(); got != 40 {
		t.Fatalf("height = %d, want 40", got)
	}
}

func TestThumbnailKeepsNarrowImages(t *testing.T) {
	original := testPNG(t, 80, 40)
	body, resized := Thumbnail(original, 160)
	if resized {
		t.Fatal("narrow image must not be upscaled")
	}
	if !bytes.Equal(body, original) {
		t.Fatal("original bytes changed")
	}
}

func TestThumbnailKeepsUndecodableBodies(t *testing.T) {
	original := []byte("not an image")
	body, resized := Thumbnail(original, 80)
	if resized || !bytes.Equal(body, original) {
		t.Fatal("undecodable body must be returned unchanged")
	}
}
