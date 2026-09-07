package codeid

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{name: "standard", input: "SSIS-589.mkv", want: "SSIS-589", ok: true},
		{name: "prefix noise", input: "[XLD]  SSIS-589 1080p.mkv", want: "SSIS-589", ok: true},
		{name: "website prefix", input: "hhd800.com@SSIS001.mp4", want: "SSIS-001", ok: true},
		{name: "bracketed website", input: "[hhd800.com]SSIS001.mp4", want: "SSIS-001", ok: true},
		{name: "codec prefix", input: "H.264 SSIS-589.mp4", want: "SSIS-589", ok: true},
		{name: "codec after compact number", input: "SSIS001 x265.mkv", want: "SSIS-001", ok: true},
		{name: "western scene", input: "RKPrime.26.09.05.mp4", want: "RKPRIME.26.09.05", ok: true},
		{name: "western scene with quality suffix", input: "H.265 RKPrime.26.09.05.1080p.mkv", want: "RKPRIME.26.09.05", ok: true},
		{name: "western next day stays distinct", input: "RKPrime.26.09.06.mp4", want: "RKPRIME.26.09.06", ok: true},
		{name: "western long site name", input: "ExampleStudioName.26.09.05.mp4", want: "EXAMPLESTUDIONAME.26.09.05", ok: true},
		{name: "western four digit year", input: "ExampleStudio.2026.09.05.mp4", want: "EXAMPLESTUDIO.2026.09.05", ok: true},
		{name: "bare dotted date is not a code", input: "26.09.05.mp4", want: "", ok: false},
		{name: "episode suffix", input: "SSIS-589-02.mp4", want: "SSIS-589", ok: true},
		{name: "subtitle suffix", input: "SSIS-589-C [中字].mp4", want: "SSIS-589", ok: true},
		{name: "fc2", input: "FC2-PPV-1234567.mp4", want: "FC2-PPV-1234567", ok: true},
		{name: "fc2 compact", input: "fc2ppv1234567", want: "FC2-PPV-1234567", ok: true},
		{name: "fc2 without ppv", input: "FC2_1234567", want: "FC2-PPV-1234567", ok: true},
		{name: "compact", input: "ssis001.mkv", want: "SSIS-001", ok: true},
		{name: "compact digit prefix", input: "1PONDO123456", want: "1PONDO-123456", ok: true},
		{name: "unicode dash", input: "IPX－001.mkv", want: "IPX-001", ok: true},
		{name: "single letter prefix", input: "A-1023", want: "A-1023", ok: true},
		{name: "digit prefix separated", input: "1PONDO-123456.mp4", want: "1PONDO-123456", ok: true},
		{name: "numeric catalogue", input: "011015-780.mp4", want: "011015-780", ok: true},
		{name: "numeric catalogue underscore", input: "072625_01.mp4", want: "072625-01", ok: true},
		{name: "studio numeric segment one", input: "1pondo-123456_01.mp4", want: "1PONDO-123456-01", ok: true},
		{name: "studio numeric segment two", input: "1pondo-123456_02.mp4", want: "1PONDO-123456-02", ok: true},
		{name: "compact studio numeric segment", input: "1pondo123456_02.mp4", want: "1PONDO-123456-02", ok: true},
		{name: "catalogue letter suffix", input: "FJIN-106a.mkv", want: "FJIN-106A", ok: true},
		{name: "compact catalogue letter suffix", input: "fjin106b.mp4", want: "FJIN-106B", ok: true},
		{name: "letter suffix with episode", input: "FJIN-106a-02.mp4", want: "FJIN-106A", ok: true},
		{name: "letter suffix with subtitles", input: "FJIN-106a-C.mp4", want: "FJIN-106A", ok: true},
		{name: "date is not a number", input: "2002-01-05.mp4", want: "", ok: false},
		{name: "not a number", input: "poster.jpg", want: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Parse(tt.input)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("Parse(%q) = %q, %v; want %q, %v", tt.input, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: " ssis 589 ", want: "SSIS-589"},
		{input: " RKPrime.26.09.05 ", want: "RKPRIME.26.09.05"},
		{input: "RKPrime.26.09.06", want: "RKPRIME.26.09.06"},
		{input: "ExampleStudioName.26.09.05", want: "EXAMPLESTUDIONAME.26.09.05"},
		{input: "21Studio.26.09.05", want: "21STUDIO.26.09.05"},
		{input: "ExampleStudio.2026.09.05", want: "EXAMPLESTUDIO.2026.09.05"},
		{input: "RKPrime.26.09.05.mp4", want: ""},
		{input: "RKPrime.26.09.05.1080p", want: ""},
		{input: "26.09.05", want: ""},
		{input: "FC2 PPV 1234567", want: "FC2-PPV-1234567"},
		{input: "FC2 PPV 1234567-C", want: ""},
		{input: "1pondo-123456_01", want: "1PONDO-123456-01"},
		{input: "1pondo-123456_02", want: "1PONDO-123456-02"},
		{input: "011015-780", want: "011015-780"},
		{input: "072625_01", want: "072625-01"},
		{input: "SSIS-589-02", want: "SSIS-589-02"},
		{input: "SSIS-589.mp4", want: ""},
		{input: "prefix SSIS-589", want: ""},
		{input: "2002-01-05", want: ""},
		{input: "fjin-106a", want: "FJIN-106A"},
		{input: "FJIN-106", want: "FJIN-106"},
		{input: "not-a-code", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := Normalize(tt.input); got != tt.want {
				t.Fatalf("Normalize(%q) = %q; want %q", tt.input, got, tt.want)
			}
			if tt.want != "" && Normalize(tt.want) != tt.want {
				t.Fatalf("Normalize is not idempotent for %q", tt.want)
			}
		})
	}
}
