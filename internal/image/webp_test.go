package image

import (
	"bytes"
	"encoding/base64"
	stdimage "image"
	"testing"
)

func TestWebPArtwork(t *testing.T) {
	// Synthetic 6x4 red/blue images, including VP8X containers with EXIF
	// metadata. Do not import the decoder here: production must register it.
	for _, fixture := range []struct{ name, encoded string }{
		{"lossy", "UklGRloAAABXRUJQVlA4IE4AAABwAwCdASoGAAQAAMASJagCdLoB+AFGA/ACtAP4BlAALNrLqIAA/vwLiFIcP/7h+Faf4N/3QJ9h+Faf4N//dAn/xLapPa8nBY6b1p7hgAA="},
		{"lossless", "UklGRiAAAABXRUJQVlA4TBQAAAAvBcAAAA9QkAOBHwR4/qNCcSL6Hw=="},
		{"VP8X lossy with EXIF", "UklGRrAAAABXRUJQVlA4WAoAAAAIAAAABQAAAwAAVlA4IE4AAABwAwCdASoGAAQAAMASJagCdLoB+AFGA/ACtAP4BlAALNrLqIAA/vwLiFIcP/7h+Faf4N/3QJ9h+Faf4N//dAn/xLapPa8nBY6b1p7hgABFWElGPAAAAE1NACoAAAAIAAEBDgACAAAAIgAAABoAAAAAU3ludGhldGljIFdlYlAgcmVncmVzc2lvbiBmaXh0dXJlAA=="},
		{"VP8X lossless with EXIF", "UklGRnYAAABXRUJQVlA4WAoAAAAIAAAABQAAAwAAVlA4TBQAAAAvBcAAAA9QkAOBHwR4/qNCcSL6H0VYSUY8AAAATU0AKgAAAAgAAQEOAAIAAAAiAAAAGgAAAABTeW50aGV0aWMgV2ViUCByZWdyZXNzaW9uIGZpeHR1cmUA"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			body, err := base64.StdEncoding.DecodeString(fixture.encoded)
			if err != nil {
				t.Fatal(err)
			}
			for _, source := range []string{"cover", "restore"} {
				t.Run(source, func(t *testing.T) {
					cache, err := NewCache(t.TempDir())
					if err != nil {
						t.Fatal(err)
					}
					var artwork Artwork
					posterSize := stdimage.Pt(6, 4)
					if source == "cover" {
						artwork, err = cache.FromCover(body)
						posterSize = stdimage.Pt(2, 4)
					} else {
						artwork, err = cache.Restore(body, body)
					}
					if err != nil {
						t.Fatalf("process WebP artwork: %v", err)
					}
					for _, output := range []struct {
						name, url string
						size      stdimage.Point
					}{
						{"poster", artwork.Poster, posterSize},
						{"fanart", artwork.Fanart, stdimage.Pt(6, 4)},
						{"thumbnail", artwork.Thumbnail, stdimage.Pt(480, 320)},
					} {
						data, err := cache.ReadURL(output.url)
						if err != nil {
							t.Fatal(err)
						}
						decoded, format, err := stdimage.Decode(bytes.NewReader(data))
						if err != nil {
							t.Fatalf("decode %s: %v", output.name, err)
						}
						if format != "jpeg" || decoded.Bounds().Size() != output.size {
							t.Errorf("%s = %s %v, want jpeg %v", output.name, format, decoded.Bounds().Size(), output.size)
						}
					}
				})
			}
		})
	}
}
