package subtitle

import (
	"bytes"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestDecodeToUTF8(t *testing.T) {
	t.Run("utf-8 with BOM", func(t *testing.T) {
		bom := []byte{0xEF, 0xBB, 0xBF}
		raw := append(bom, []byte("你好世界")...)
		got, err := DecodeToUTF8(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got != "你好世界" {
			t.Fatalf("got %q, want %q", got, "你好世界")
		}
	})

	t.Run("GBK encoded string", func(t *testing.T) {
		gbkEncoder := simplifiedchinese.GB18030.NewEncoder()
		gbkBytes, err := gbkEncoder.Bytes([]byte("中文字幕测试"))
		if err != nil {
			t.Fatal(err)
		}

		got, err := DecodeToUTF8(gbkBytes)
		if err != nil {
			t.Fatal(err)
		}
		if got != "中文字幕测试" {
			t.Fatalf("got %q, want %q", got, "中文字幕测试")
		}
	})
}

func TestNormalizeKeepsTheFormatAndWritesUTF8(t *testing.T) {
	srt := "1\r\n00:01:20,000 --> 00:01:23,500\r\n你好，世界！\r\n"
	encoded, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(srt))
	if err != nil {
		t.Fatal(err)
	}
	body, text, err := Normalize(encoded, "srt")
	if err != nil {
		t.Fatal(err)
	}
	if text != srt || !bytes.Equal(body, append([]byte{0xEF, 0xBB, 0xBF}, srt...)) {
		t.Fatalf("SRT was not preserved as UTF-8 with a BOM: %q", body)
	}

	ass := "[Script Info]\nTitle: Fixture\n\n[Events]\nFormat: Layer, Start, End, Style, Text\n" +
		`Dialogue: 0,0:00:01.00,0:00:02.00,Default,{\an8}你好` + "\n"
	if _, text, err := Normalize([]byte(ass), "ass"); err != nil || text != ass {
		t.Fatalf("ASS styling was not preserved: %q, %v", text, err)
	}
	// Loose SRT timing written by some tools stays valid.
	if _, _, err := Normalize([]byte("1\n0:01:20.5 --> 0:01:23.5\nHi\n"), "srt"); err != nil {
		t.Fatalf("loose SRT timing rejected: %v", err)
	}
}

func TestNormalizeRejectsPayloadsWithoutCues(t *testing.T) {
	for _, tt := range []struct{ name, content, format string }{
		{"json error", `{"status": 500, "message": "internal server error"}`, "srt"},
		{"html error", `<!DOCTYPE html><html><body>Access Denied</body></html>`, "srt"},
		{"empty content", ``, "srt"},
		{"plain text", `this is just some plain text without any timestamps`, "vtt"},
		{"ass without dialogue", "[Script Info]\nTitle: Empty\n", "ass"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := Normalize([]byte(tt.content), tt.format); err == nil {
				t.Errorf("accepted %s", tt.name)
			}
		})
	}
}
