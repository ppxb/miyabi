package providers

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

var charsetDecl = regexp.MustCompile(`(?i)charset\s*=\s*["']?([a-z0-9_\-]+)`)

// decodeHTML converts legacy Japanese encodings to UTF-8. Several P1 amateur
// sites serve EUC-JP (some only via the HTTP header, which client.get does not
// expose), so when no declaration is present it falls back on the byte stream:
// valid UTF-8 is kept as-is, anything else is decoded as EUC-JP.
func decodeHTML(body []byte) string {
	head := body
	if len(head) > 4096 {
		head = head[:4096]
	}
	enc := ""
	if m := charsetDecl.FindSubmatch(head); m != nil {
		enc = strings.ToLower(string(m[1]))
	}
	switch {
	case strings.Contains(enc, "euc"):
		return decodeWith(japanese.EUCJP.NewDecoder(), body)
	case strings.Contains(enc, "shift_jis") || strings.Contains(enc, "sjis"):
		return decodeWith(japanese.ShiftJIS.NewDecoder(), body)
	}
	if utf8.Valid(body) {
		return string(body)
	}
	return decodeWith(japanese.EUCJP.NewDecoder(), body)
}

func decodeWith(t transform.Transformer, body []byte) string {
	if decoded, _, err := transform.Bytes(t, body); err == nil {
		return string(decoded)
	}
	return string(body)
}
