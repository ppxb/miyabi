package image

import (
	"bytes"

	"github.com/disintegration/imaging"
)

// Thumbnail scales body down to width, preserving the aspect ratio, and
// re-encodes it as JPEG. It returns resized=false for images that are already
// narrow enough or cannot be decoded, so the caller serves the original bytes.
func Thumbnail(body []byte, width int) ([]byte, bool) {
	source, err := imaging.Decode(bytes.NewReader(body), imaging.AutoOrientation(true))
	if err != nil || source.Bounds().Dx() <= width {
		return body, false
	}
	resized := imaging.Resize(source, width, 0, imaging.Lanczos)
	var buffer bytes.Buffer
	if err := imaging.Encode(&buffer, resized, imaging.JPEG, imaging.JPEGQuality(85)); err != nil {
		return body, false
	}
	return buffer.Bytes(), true
}
