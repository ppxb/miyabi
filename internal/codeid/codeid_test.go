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
		{input: "FC2 PPV 1234567-C", want: "FC2-PPV-1234567"},
		{input: "fjin-106a", want: "FJIN-106A"},
		{input: "FJIN-106", want: "FJIN-106"},
		{input: "not-a-code", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := Normalize(tt.input); got != tt.want {
				t.Fatalf("Normalize(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}
