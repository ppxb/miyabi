package image

import (
	"bytes"
	stdimage "image"
	"image/jpeg"
	"testing"
)

func TestPreviewThumbnailFitsWithoutUpscalingOrChangingOriginal(t *testing.T) {
	for _, size := range []stdimage.Point{{X: 1280, Y: 720}, {X: 534, Y: 800}, {X: 100, Y: 60}} {
		var input bytes.Buffer
		if err := jpeg.Encode(&input, stdimage.NewRGBA(stdimage.Rect(0, 0, size.X, size.Y)), nil); err != nil {
			t.Fatal(err)
		}
		original := bytes.Clone(input.Bytes())
		thumbnail, err := PreviewThumbnail(input.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		config, format, err := stdimage.DecodeConfig(bytes.NewReader(thumbnail))
		if err != nil || format != "jpeg" || config.Width > PreviewThumbnailSize || config.Height > PreviewThumbnailSize || config.Width > size.X || config.Height > size.Y {
			t.Fatalf("thumbnail for %v = %+v, %s, %v", size, config, format, err)
		}
		if !bytes.Equal(original, input.Bytes()) {
			t.Fatal("changed original bytes")
		}
		if size.X <= PreviewThumbnailSize && size.Y <= PreviewThumbnailSize && (config.Width != size.X || config.Height != size.Y) {
			t.Fatal("resized a small original")
		}
		if delta := config.Width*size.Y - config.Height*size.X; delta > size.X || delta < -size.X {
			t.Fatalf("changed aspect ratio: %v -> %+v", size, config)
		}
	}
	if _, err := PreviewThumbnail([]byte("not an image")); err == nil {
		t.Fatal("accepted invalid image")
	}
}
