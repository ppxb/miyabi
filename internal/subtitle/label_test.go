package subtitle

import "testing"

func TestDetectVersion(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     VersionTag
	}{
		{name: "standard", filename: "ABP-001.chs.srt", want: VersionStandard},
		{name: "uncensored english", filename: "ABP-001.uncensored.srt", want: VersionUncensored},
		{name: "uncensored chinese", filename: "ABP-001.无码破解.srt", want: VersionUncensored},
		{name: "uncensored mosaic", filename: "ABP-001.去mosaic.srt", want: VersionUncensored},
		{name: "leaked", filename: "ABP-001.流出完整版.srt", want: VersionLeaked},
		{name: "extended", filename: "ABP-001.extended.cut.srt", want: VersionExtended},
		{name: "extended chinese", filename: "ABP-001.加长版.srt", want: VersionExtended},
		{name: "uncensored suffix", filename: "SSIS-589-U.mp4", want: VersionUncensored},
		{name: "uncensored subtitled suffix", filename: "SSIS-589-UC.mp4", want: VersionUncensored},
		{name: "subtitled suffix", filename: "SSIS-589-C.mp4", want: VersionStandard},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectVersion(tt.filename)
			if got != tt.want {
				t.Errorf("DetectVersion(%q) = %q; want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name, hint, text string
		want             Language
	}{
		{name: "hint zh-tw", hint: "zh-TW", want: LangTraditionalChinese},
		{name: "hint chs in file name", hint: "ABP-001.chs.srt", want: LangSimplifiedChinese},
		{name: "hint cht wins over text", hint: "ABP-001.cht.ass", text: "这是一个关于开发的问题", want: LangTraditionalChinese},
		{name: "simplified text", text: "这是一个关于开发的问题，这个开关还在这里。", want: LangSimplifiedChinese},
		{name: "traditional text", text: "這是一個關於開發的問題，這個開關還在這裡。", want: LangTraditionalChinese},
		{name: "no evidence", want: LangSimplifiedChinese},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectLanguage(tt.hint, tt.text); got != tt.want {
				t.Errorf("DetectLanguage(%q, %q) = %q; want %q", tt.hint, tt.text, got, tt.want)
			}
		})
	}
	// A code prefix must not read as a language marker.
	if got := LanguageHint("SSIS-589.srt"); got != LangUnknown {
		t.Errorf("LanguageHint without markers = %q", got)
	}
}

func TestHasHardSubtitle(t *testing.T) {
	for name, want := range map[string]bool{
		"SSIS-589-C.mp4":   true,
		"SSIS-589-UC.mkv":  true,
		"SSIS-589_ch.mp4":  true,
		"SSIS-589 中字.mp4":  true,
		"SSIS-589.mp4":     false,
		"SSIS-589-CD1.mp4": false,
		"CAWD-123.mp4":     false,
		"SSIS-589-4K.mp4":  false,
	} {
		if got := HasHardSubtitle(name); got != want {
			t.Errorf("HasHardSubtitle(%q) = %v; want %v", name, got, want)
		}
	}
}

func TestIsUncensored(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"ABP-123.uncensored.mp4", true},
		{"ABP-123.无码.mp4", true},
		{"ABP-123.無碼.mp4", true},
		{"ABP-123.破解.mp4", true},
		{"ABP-123.流出.mp4", true},
		{"ABP-123.leaked.mp4", true},
		{"ABP-123.mosaic.mp4", true},
		{"ABP-123.standard.mp4", false},
		{"ABP-123.chs.srt", false},
		{"ABP-123-UC.mp4", true},
		{"ABP-123-C.mp4", false},
	}

	for _, tc := range cases {
		if got := IsUncensored(tc.name); got != tc.want {
			t.Errorf("IsUncensored(%q) = %v; want %v", tc.name, got, tc.want)
		}
	}
}
