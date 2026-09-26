package codeid

import (
	"slices"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{name: "standard", input: "SSIS-589.mkv", want: "SSIS-589", ok: true},
		{name: "label digits stay intact", input: "259LUXU-1899.mp4", want: "259LUXU-1899", ok: true},
		{name: "compact label digits with disc", input: "259luxu1899-CD1.mp4", want: "259LUXU-1899", ok: true},
		{name: "label digits with subtitle", input: "[example.com] 326IHD_005-C.mkv", want: "326IHD-005", ok: true},
		{name: "four digit sequence", input: "GLOD-0436.mp4", want: "GLOD-0436", ok: true},
		{name: "letter before sequence", input: "KNB-M014.mp4", want: "KNB-M014", ok: true},
		{name: "letter serial is not a standalone disc marker", input: "KNB-CD014.mp4", want: "KNB-CD014", ok: true},
		{name: "letter sequence underscore", input: "knb_m014.mkv", want: "KNB-M014", ok: true},
		{name: "letter sequence video part", input: "KNB-M014-CD1.mp4", want: "KNB-M014", ok: true},
		{name: "compact number video part", input: "abp001-CD1.mp4", want: "ABP-001", ok: true},
		{name: "compact number named part", input: "abp001-PART2.mkv", want: "ABP-001", ok: true},
		{name: "alphanumeric sequence", input: "KNB-M014A2-C.mp4", want: "KNB-M014A2", ok: true},
		{name: "unknown prefix does not expose partial code", input: "UNKNOWN-KNB-M014.mp4", want: "", ok: false},
		{name: "unknown dotted prefix does not expose partial code", input: "UNKNOWN.KNB-M014.mp4", want: "", ok: false},
		{name: "extension is not a letter sequence", input: "video.m014", want: "", ok: false},
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
		{name: "compact SSNI subtitle", input: "SSNI748C.mp4", want: "SSNI-748", ok: true},
		{name: "attached SSNI subtitle", input: "ssni-748c.mkv", want: "SSNI-748", ok: true},
		{name: "SSNI subtitle and video part", input: "SSNI748C-CD2.mp4", want: "SSNI-748", ok: true},
		{name: "other catalogue C variant stays distinct", input: "FJIN-106C.mp4", want: "FJIN-106C", ok: true},
		{name: "title suffix", input: "SSIS-589-Example-Title.mp4", want: "SSIS-589", ok: true},
		{name: "named catalogue", input: "scute-1575-itsuki.mp4", want: "SCUTE-1575-ITSUKI", ok: true},
		{name: "different catalogue name", input: "scute-1575-nanami.mp4", want: "SCUTE-1575-NANAMI", ok: true},
		{name: "compact named catalogue", input: "scute1575_itsuki.mkv", want: "SCUTE-1575-ITSUKI", ok: true},
		{name: "named catalogue with digits", input: "scute-1575-itsuki2.mp4", want: "SCUTE-1575-ITSUKI2", ok: true},
		{name: "multiple name segments", input: "scute-1575-example-name.mp4", want: "SCUTE-1575-EXAMPLE-NAME", ok: true},
		{name: "named catalogue with video part", input: "SCUTE-1575-ITSUKI-02.mp4", want: "SCUTE-1575-ITSUKI", ok: true},
		{name: "named catalogue with disc", input: "SCUTE-1575-ITSUKI-CD1.mp4", want: "SCUTE-1575-ITSUKI", ok: true},
		{name: "named catalogue with subtitles", input: "[example.com] SCUTE－1575－ITSUKI-C H.265.mkv", want: "SCUTE-1575-ITSUKI", ok: true},
		{name: "named catalogue with quality", input: "SCUTE-1575-ITSUKI-UHD-2160p.mkv", want: "SCUTE-1575-ITSUKI", ok: true},
		{name: "named catalogue with title", input: "SCUTE-1575-ITSUKI Example Title.mp4", want: "SCUTE-1575-ITSUKI", ok: true},
		{name: "named catalogue sidecar", input: "SCUTE-1575-ITSUKI.nfo", want: "SCUTE-1575-ITSUKI", ok: true},
		{name: "subtitle is not a catalogue name", input: "SCUTE-1575-C.mp4", want: "SCUTE-1575", ok: true},
		{name: "disc is not a catalogue name", input: "SCUTE-1575-CD1.mp4", want: "SCUTE-1575", ok: true},
		{name: "fc2", input: "FC2-PPV-1234567.mp4", want: "FC2-PPV-1234567", ok: true},
		{name: "fc2 compact", input: "fc2ppv1234567", want: "FC2-PPV-1234567", ok: true},
		{name: "fc2 without ppv", input: "FC2_1234567", want: "FC2-PPV-1234567", ok: true},
		{name: "compact", input: "ssis001.mkv", want: "SSIS-001", ok: true},
		{name: "compact digit prefix", input: "1PONDO123456", want: "1PONDO-123456", ok: true},
		{name: "unicode dash", input: "IPX－001.mkv", want: "IPX-001", ok: true},
		{name: "single letter prefix", input: "A-1023", want: "A-1023", ok: true},
		{name: "single digit number", input: "SSIS-1.mp4", want: "SSIS-1", ok: true},
		{name: "long prefix", input: "ExampleStudioName-001.mkv", want: "EXAMPLESTUDIONAME-001", ok: true},
		{name: "long sequence", input: "EXAMPLE-1234567890.mp4", want: "EXAMPLE-1234567890", ok: true},
		{name: "digit prefix separated", input: "1PONDO-123456.mp4", want: "1PONDO-123456", ok: true},
		{name: "prefix containing digits", input: "T28-638.mp4", want: "T28-638", ok: true},
		{name: "digit prefix with four digit number", input: "T28-6301-CD1.mp4", want: "T28-6301", ok: true},
		{name: "numeric catalogue", input: "011015-780.mp4", want: "011015-780", ok: true},
		{name: "numeric catalogue underscore", input: "072625_01.mp4", want: "072625_01", ok: true},
		{name: "studio numeric segment one", input: "1pondo-123456_01.mp4", want: "1PONDO-123456-01", ok: true},
		{name: "studio numeric segment two", input: "1pondo-123456_02.mp4", want: "1PONDO-123456-02", ok: true},
		{name: "compact studio numeric segment", input: "1pondo123456_02.mp4", want: "1PONDO-123456-02", ok: true},
		{name: "heydouga catalogue", input: "heydouga-4030-2347.mp4", want: "HEYDOUGA-4030-2347", ok: true},
		{name: "heydouga next movie stays distinct", input: "heydouga-4030-2348.mp4", want: "HEYDOUGA-4030-2348", ok: true},
		{name: "heydouga compact prefix", input: "heydouga4030_2347.mkv", want: "HEYDOUGA-4030-2347", ok: true},
		{name: "heydouga dotted separator", input: "heydouga.4030.2347.mp4", want: "HEYDOUGA-4030-2347", ok: true},
		{name: "heydouga spaced separator", input: "HEYDOUGA 4030 2347.mp4", want: "HEYDOUGA-4030-2347", ok: true},
		{name: "heydouga padded sequence", input: "HEYDOUGA-4017-00001.mp4", want: "HEYDOUGA-4017-00001", ok: true},
		{name: "heydouga short sequence", input: "HEYDOUGA-4017-1.mp4", want: "HEYDOUGA-4017-1", ok: true},
		{name: "heydouga numeric video part", input: "HEYDOUGA-4030-2347-02.mp4", want: "HEYDOUGA-4030-2347", ok: true},
		{name: "heydouga named video part", input: "HEYDOUGA-4030-2347-CD1.mp4", want: "HEYDOUGA-4030-2347", ok: true},
		{name: "heydouga noise and subtitles", input: "[example.com] HEYDOUGA－4030－2347-C H.265.mkv", want: "HEYDOUGA-4030-2347", ok: true},
		{name: "catalogue letter suffix", input: "FJIN-106a.mkv", want: "FJIN-106A", ok: true},
		{name: "compact catalogue letter suffix", input: "fjin106b.mp4", want: "FJIN-106B", ok: true},
		{name: "letter suffix with episode", input: "FJIN-106a-02.mp4", want: "FJIN-106A", ok: true},
		{name: "letter suffix with subtitles", input: "FJIN-106a-C.mp4", want: "FJIN-106A", ok: true},
		{name: "multiple letter variant", input: "EXAMPLE-106ab-02.mp4", want: "EXAMPLE-106AB", ok: true},
		{name: "date is not a number", input: "2002-01-05.mp4", want: "", ok: false},
		{name: "file extension is not a number", input: "video.mp4", want: "", ok: false},
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
		{input: "259LUXU-1899", want: "259LUXU-1899"},
		{input: "259luxu1899", want: "259LUXU-1899"},
		{input: "259LUXU_1899", want: "259LUXU-1899"},
		{input: "259LUXU－1899", want: "259LUXU-1899"},
		{input: "259LUXU-01899", want: "259LUXU-01899"},
		{input: "259LUXU-1899-C", want: "259LUXU-1899-C"},
		{input: "LUXU-1899", want: "LUXU-1899"},
		{input: "999LUXU-1899", want: "999LUXU-1899"},
		{input: "1259LUXU-1899", want: "1259LUXU-1899"},
		{input: "259LUXUS-1899", want: "259LUXUS-1899"},
		{input: "10MUSUME-123456-01", want: "10MUSUME-123456-01"},
		{input: "1000GIRI-001", want: "1000GIRI-001"},
		{input: "GLOD-0436", want: "GLOD-0436"},
		{input: "KNB-M014", want: "KNB-M014"},
		{input: "knb_m014", want: "KNB-M014"},
		{input: "KNB M014", want: "KNB-M014"},
		{input: "KNB-M014A2", want: "KNB-M014A2"},
		{input: "M-014", want: "M-014"},
		{input: " RKPrime.26.09.05 ", want: "RKPRIME.26.09.05"},
		{input: "RKPrime.26.09.06", want: "RKPRIME.26.09.06"},
		{input: "ExampleStudioName.26.09.05", want: "EXAMPLESTUDIONAME.26.09.05"},
		{input: "21Studio.26.09.05", want: "21STUDIO.26.09.05"},
		{input: "ExampleStudio.2026.09.05", want: "EXAMPLESTUDIO.2026.09.05"},
		{input: "ExampleStudio.26.09.05-scene2", want: "EXAMPLESTUDIO.26.09.05-SCENE2"},
		{input: "RKPrime.26.09.05.mp4", want: "RKPRIME.26.09.05.MP4"},
		{input: "RKPrime.26.09.05.1080p", want: "RKPRIME.26.09.05.1080P"},
		{input: "26.09.05", want: "26.09.05"},
		{input: "FC2 PPV 1234567", want: "FC2-PPV-1234567"},
		{input: "FC2 PPV 1234567-C", want: "FC2-PPV-1234567-C"},
		{input: "FC2PPV1234567890", want: "FC2-PPV-1234567890"},
		{input: " scute-1575-itsuki ", want: "SCUTE-1575-ITSUKI"},
		{input: "scute_1575_nanami", want: "SCUTE-1575-NANAMI"},
		{input: "SCUTE－1575－ITSUKI", want: "SCUTE-1575-ITSUKI"},
		{input: "scute1575_itsuki", want: "SCUTE-1575-ITSUKI"},
		{input: "SCUTE-1575-ITSUKI2", want: "SCUTE-1575-ITSUKI2"},
		{input: "SCUTE-1575-ITSUKI-C", want: "SCUTE-1575-ITSUKI-C"},
		{input: "SCUTE-1575-ITSUKI-02", want: "SCUTE-1575-ITSUKI-02"},
		{input: "EXAMPLE-123-model-name-0002", want: "EXAMPLE-123-MODEL-NAME-0002"},
		{input: "EXAMPLE-123-A1B2", want: "EXAMPLE-123-A1B2"},
		{input: "EXAMPLE-123-2A3B", want: "EXAMPLE-123-2A3B"},
		{input: "EXAMPLE-123-A", want: "EXAMPLE-123-A"},
		{input: "EXAMPLE-123ab", want: "EXAMPLE-123AB"},
		{input: "ExampleStudioName-001", want: "EXAMPLESTUDIONAME-001"},
		{input: "EXAMPLE-1234567890", want: "EXAMPLE-1234567890"},
		{input: "SSIS-1", want: "SSIS-1"},
		{input: "1pondo-123456_01", want: "1PONDO-123456-01"},
		{input: "1pondo-123456_02", want: "1PONDO-123456-02"},
		{input: "1pondo123456_02", want: "1PONDO-123456-02"},
		{input: "T28-638", want: "T28-638"},
		{input: "T28-6301", want: "T28-6301"},
		{input: "ID2-12345", want: "ID2-12345"},
		{input: "heydouga-4030-2347", want: "HEYDOUGA-4030-2347"},
		{input: "heydouga-4030-2348", want: "HEYDOUGA-4030-2348"},
		{input: "heydouga4030_2347", want: "HEYDOUGA-4030-2347"},
		{input: "heydouga.4030.2347", want: "HEYDOUGA-4030-2347"},
		{input: " HEYDOUGA 4030 2347 ", want: "HEYDOUGA-4030-2347"},
		{input: "HEYDOUGA－4030－2347", want: "HEYDOUGA-4030-2347"},
		{input: "HEYDOUGA-4017-00001", want: "HEYDOUGA-4017-00001"},
		{input: "HEYDOUGA-4017-1", want: "HEYDOUGA-4017-1"},
		{input: "HEYDOUGA-4030-2347-02", want: "HEYDOUGA-4030-2347-02"},
		{input: "HEYDOUGA4030_2347_02", want: "HEYDOUGA-4030-2347-02"},
		{input: "EXAMPLE-123-0000042-09", want: "EXAMPLE-123-0000042-09"},
		{input: "HEYDOUGA-4030-2347.mp4", want: "HEYDOUGA-4030-2347.MP4"},
		{input: "HEYDOUGA-4030-2347-CD1", want: "HEYDOUGA-4030-2347-CD1"},
		{input: "HEYDOUGA-4030-2347-C", want: "HEYDOUGA-4030-2347-C"},
		{input: "011015-780", want: "011015-780"},
		{input: "072625_01", want: "072625_01"},
		{input: "SSIS-589-02", want: "SSIS-589-02"},
		{input: "SSIS-589.mp4", want: "SSIS-589.MP4"},
		{input: "SCUTE-1575-ITSUKI.mp4", want: "SCUTE-1575-ITSUKI.MP4"},
		{input: "SCUTE-1575-ITSUKI.mkv", want: "SCUTE-1575-ITSUKI.MKV"},
		{input: "SCUTE-1575-ITSUKI Example Title", want: "SCUTE-1575-ITSUKI EXAMPLE TITLE"},
		{input: "SCUTE-1575-ITSUKI/other", want: "SCUTE-1575-ITSUKI/OTHER"},
		{input: "SCUTE-1575-", want: "SCUTE-1575-"},
		{input: "prefix SSIS-589", want: "PREFIX SSIS-589"},
		{input: "2002-01-05", want: "2002-01-05"},
		{input: "fjin-106a", want: "FJIN-106A"},
		{input: "SSNI748C", want: "SSNI-748C"},
		{input: "FJIN-106", want: "FJIN-106"},
		{input: "not-a-code", want: "NOT-A-CODE"},
		{input: " 配信/作品 #0007 ", want: "配信/作品 #0007"},
		{input: "", want: ""},
		{input: " \t\n", want: ""},
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

func FuzzNormalizeRetainsCompleteNumbers(f *testing.F) {
	for _, input := range []string{"KNB-M014", "M-014", "SCUTE-1575-ITSUKI", "ExampleStudio.26.09.05", "作品/限定 #0007", ""} {
		f.Add(input)
	}
	f.Fuzz(func(t *testing.T, input string) {
		code := Normalize(input)
		if strings.TrimSpace(input) != "" && code == "" {
			t.Fatal("discarded a nonempty complete number")
		}
		if Normalize(code) != code {
			t.Fatalf("normalization is not idempotent: %q -> %q -> %q", input, code, Normalize(code))
		}
	})
}

func TestIsEquivalent(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{name: "identical", a: "SSIS-589", b: "SSIS-589", want: true},
		{name: "case insensitive", a: "ssis-589", b: "SSIS-589", want: true},
		{name: "punctuation normalized", a: "SSIS_589", b: "SSIS-589", want: true},
		{name: "fc2 normalization", a: "FC2-1234567", b: "FC2-PPV-1234567", want: true},
		{name: "distributor prefix 200GANA vs GANA", a: "200GANA-3458", b: "GANA-3458", want: true},
		{name: "distributor prefix reverse order", a: "GANA-3458", b: "200GANA-3458", want: true},
		{name: "distributor prefix 259LUXU vs LUXU", a: "259LUXU-1899", b: "LUXU-1899", want: true},
		{name: "distributor prefix 300MIUM vs MIUM", a: "300MIUM-001", b: "MIUM-001", want: true},
		{name: "studio prefix with date code", a: "CARIB-060326-001", b: "060326-001", want: true},
		{name: "studio prefix with date code reverse", a: "060326-001", b: "CARIB-060326-001", want: true},
		{name: "1pondo prefix with date code", a: "1PONDO-060326-001", b: "060326-001", want: true},
		{name: "padding zeros", a: "ABC-00123", b: "abc123", want: true},
		{name: "distributor prefix with padding", a: "326IHD-005", b: "IHD-5", want: true},
		{name: "reject different distributor digits", a: "259LUXU-1899", b: "999LUXU-1899", want: false},
		{name: "reject different studios same date code", a: "CARIB-060326-001", b: "1PONDO-060326-001", want: false},
		{name: "reject letter serial variant", a: "FJIN-106", b: "FJIN-106C", want: false},
		{name: "reject different numbers", a: "GANA-3458", b: "GANA-3459", want: false},
		{name: "reject different studios same number", a: "IPX-123", b: "SSIS-123", want: false},
		{name: "reject non-digit prefix suffix", a: "AB-123", b: "B-123", want: false},
		{name: "reject non-date code without prefix", a: "ABC-12345", b: "12345", want: false},
		{name: "reject empty a", a: "", b: "SSIS-589", want: false},
		{name: "reject empty b", a: "SSIS-589", b: "", want: false},
		{name: "reject both empty", a: "", b: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEquivalent(tt.a, tt.b); got != tt.want {
				t.Errorf("IsEquivalent(%q, %q) = %v; want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestIsFormatEquivalent(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{name: "identical", a: "SSIS-589", b: "SSIS-589", want: true},
		{name: "compact vs hyphenated with leading zeros", a: "ABC00123", b: "ABC-123", want: true},
		{name: "different padding zeros", a: "ABP-001", b: "ABP-1", want: true},
		{name: "three digit padding vs two digit", a: "IPX-052", b: "IPX-52", want: true},
		{name: "delimiter underscore", a: "IPX_052", b: "ipx-52", want: true},
		{name: "zero value sequence", a: "ABC-000", b: "ABC-0", want: true},
		{name: "reject different numbers", a: "ABC-123", b: "ABC-124", want: false},
		{name: "reject substring number truncated", a: "ABC-12", b: "ABC-123", want: false},
		{name: "reject letter suffix variant", a: "FJIN-106", b: "FJIN-106A", want: false},
		{name: "reject different letter variants", a: "FJIN-106A", b: "FJIN-106B", want: false},
		{name: "reject alphanumeric serial truncation", a: "KNB-M01", b: "KNB-M014", want: false},
		{name: "reject different studios same number", a: "ABC-123", b: "DEF-123", want: false},
		{name: "reject distributor prefix difference", a: "200GANA-3458", b: "GANA-3458", want: false},
		{name: "reject empty", a: "", b: "ABC-123", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsFormatEquivalent(tt.a, tt.b); got != tt.want {
				t.Errorf("IsFormatEquivalent(%q, %q) = %v; want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCandidates(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{input: "SSIS-589", want: []string{"SSIS-589"}},
		{input: "326ihd005", want: []string{"326IHD-005", "IHD-005"}},
		{input: "200GANA-3458", want: []string{"200GANA-3458", "GANA-3458"}},
		{input: "259LUXU-1899-C", want: []string{"259LUXU-1899-C", "LUXU-1899-C"}},
		{input: "1000GIRI-001", want: []string{"1000GIRI-001", "GIRI-001"}},
		{input: "CARIB-060326-001", want: []string{"CARIB-060326-001", "060326-001"}},
		{input: "1pondo-060326_001", want: []string{"1PONDO-060326-001", "060326-001"}},
		{input: "060326-001", want: []string{"060326-001"}},
		{input: "T28-638", want: []string{"T28-638"}},
		{input: "FC2-PPV-1234567", want: []string{"FC2-PPV-1234567"}},
		{input: "HEYDOUGA-4030-2347", want: []string{"HEYDOUGA-4030-2347"}},
		{input: "21Studio.26.09.05-scene2", want: []string{"21STUDIO.26.09.05-SCENE2"}},
		{input: "", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := Candidates(tt.input); !slices.Equal(got, tt.want) {
				t.Errorf("Candidates(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestUnpaddedNumericCandidate(t *testing.T) {
	tests := []struct {
		input     string
		want      string
		wantMatch bool
	}{
		{input: "ABC-00123", want: "ABC-123", wantMatch: true},
		{input: "IPX-052", want: "IPX-52", wantMatch: true},
		{input: "ABC-123", want: "", wantMatch: false},
		{input: "ABC-0", want: "", wantMatch: false},
		{input: "ABC-00", want: "ABC-0", wantMatch: true},
		{input: "FJIN-106A", want: "", wantMatch: false},
		{input: "CARIB-060326-001", want: "", wantMatch: false},
		{input: "", want: "", wantMatch: false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := UnpaddedNumericCandidate(tt.input)
			if got != tt.want || ok != tt.wantMatch {
				t.Errorf("UnpaddedNumericCandidate(%q) = (%q, %v); want (%q, %v)", tt.input, got, ok, tt.want, tt.wantMatch)
			}
		})
	}
}

func TestPrefix(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  string
	}{
		{"IPX-123", "IPX"},
		{"ssis-456", "SSIS"},
		{"FC2-PPV-12345", "FC2"},
		{"060326-001", "OTHERS"},
		{"", "OTHERS"},
	} {
		if got := Prefix(tt.input); got != tt.want {
			t.Errorf("Prefix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
