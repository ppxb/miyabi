package image

import (
	"bytes"
	"fmt"

	"github.com/disintegration/imaging"
)

const PreviewThumbnailSize = 320

// PreviewThumbnail produces a bounded display image without changing the original.
func PreviewThumbnail(body []byte) ([]byte, error) {
	source, err := imaging.Decode(bytes.NewReader(body), imaging.AutoOrientation(true))
	if err != nil {
		return nil, fmt.Errorf("decode preview image: %w", err)
	}
	if source.Bounds().Dx() > PreviewThumbnailSize || source.Bounds().Dy() > PreviewThumbnailSize {
		source = imaging.Fit(source, PreviewThumbnailSize, PreviewThumbnailSize, imaging.Lanczos)
	}
	var output bytes.Buffer
	if err := imaging.Encode(&output, source, imaging.JPEG, imaging.JPEGQuality(82)); err != nil {
		return nil, fmt.Errorf("encode preview thumbnail: %w", err)
	}
	return output.Bytes(), nil
}
