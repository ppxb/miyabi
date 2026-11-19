package providers

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

func TestDecodeHTMLEUCJP(t *testing.T) {
	want := "出演女優 サンプル"
	encoded, _, err := transform.Bytes(japanese.EUCJP.NewEncoder(), []byte(want))
	if err != nil {
		t.Fatal(err)
	}
	// Wrap in a tiny EUC-JP document; no charset declaration -> byte-validity fallback.
	body := append([]byte("<html><body>"), encoded...)
	body = append(body, []byte("</body></html>")...)
	got := decodeHTML(body)
	if !strings.Contains(got, want) {
		t.Fatalf("decodeHTML did not decode EUC-JP: %q", got)
	}
}

func TestDecodeHTMLKeepsUTF8(t *testing.T) {
	in := []byte("<html><body>出演 サンプル</body></html>")
	if got := decodeHTML(in); got != string(in) {
		t.Fatalf("utf8 changed: %q", got)
	}
}

func TestDecodeHTMLDeclaredCharsets(t *testing.T) {
	want := "出演"
	for _, enc := range []struct {
		name string
		decl string
		body []byte
	}{
		{"euc-declared", "euc-jp", mustEncodeEUCJP("出演")},
		{"sjis-declared", "shift_jis", mustEncodeSJIS("出演")},
	} {
		body := append([]byte(`<meta charset="`+enc.decl+`"><body>`), enc.body...)
		if got := decodeHTML(body); !strings.Contains(got, want) {
			t.Fatalf("%s: decodeHTML=%q", enc.name, got)
		}
	}
}

func mustEncodeEUCJP(s string) []byte {
	b, _, _ := transform.Bytes(japanese.EUCJP.NewEncoder(), []byte(s))
	return b
}
func mustEncodeSJIS(s string) []byte {
	b, _, _ := transform.Bytes(japanese.ShiftJIS.NewEncoder(), []byte(s))
	return b
}
