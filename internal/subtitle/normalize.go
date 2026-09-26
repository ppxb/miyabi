package subtitle

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

// maxSize bounds a subtitle download or 115 read; real subtitles are far smaller.
const maxSize = 10 << 20

var (
	utf8BOM = []byte{0xEF, 0xBB, 0xBF}
	// SRT files in the wild also use VTT-style dots and omit leading zeros.
	textCue   = regexp.MustCompile(`(?m)^\s*(?:[0-9]{1,2}:)?[0-9]{1,2}:[0-9]{2}[,.][0-9]{1,3}\s*-->\s*(?:[0-9]{1,2}:)?[0-9]{1,2}:[0-9]{2}[,.][0-9]{1,3}`)
	styledCue = regexp.MustCompile(`(?im)^\s*dialogue\s*:`)
)

// Normalize re-encodes subtitle bytes as UTF-8 with a BOM, which Emby and
// its clients read without guessing the charset, and rejects payloads without
// cues such as HTML error pages. The subtitle keeps its original format.
func Normalize(raw []byte, format string) ([]byte, string, error) {
	text, err := DecodeToUTF8(raw)
	if err != nil {
		return nil, "", err
	}
	cues := textCue
	if format == "ass" || format == "ssa" {
		cues = styledCue
	}
	if !cues.MatchString(text) {
		return nil, "", errors.New("subtitle has no timed cues")
	}
	return append(bytes.Clone(utf8BOM), text...), text, nil
}

// DecodeToUTF8 converts raw subtitle bytes from UTF-8, GBK/GB18030, or Big5 into a clean UTF-8 string.
func DecodeToUTF8(raw []byte) (string, error) {
	raw = bytes.TrimPrefix(raw, utf8BOM)
	if len(raw) == 0 {
		return "", errors.New("empty subtitle data")
	}
	if utf8.Valid(raw) && !bytes.Contains(raw, []byte("\uFFFD")) {
		return string(raw), nil
	}
	// GB18030 is the most common legacy encoding of Chinese fansub releases,
	// Big5 of Traditional Chinese releases.
	if text, ok := decode(raw, simplifiedchinese.GB18030.NewDecoder()); ok && !strings.Contains(text, "\uFFFD") {
		return text, nil
	}
	if text, ok := decode(raw, traditionalchinese.Big5.NewDecoder()); ok {
		return text, nil
	}
	return string(bytes.ToValidUTF8(raw, []byte(" "))), nil
}

func decode(raw []byte, decoder transform.Transformer) (string, bool) {
	decoded, err := io.ReadAll(transform.NewReader(bytes.NewReader(raw), decoder))
	return string(decoded), err == nil && len(decoded) > 0 && utf8.Valid(decoded)
}
