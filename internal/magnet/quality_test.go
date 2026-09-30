package magnet

import (
	"slices"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
)

func TestCleanTitle(t *testing.T) {
	cases := []struct{ input, want string }{
		{"【最新APP下载】SSIS-001", "SSIS-001"},
		{"[夸克网盘发布] ABP-123", "ABP-123"},
		{"【防失联地址】MIDE-456 完整版", "MIDE-456 完整版"},
		{"NORMAL-001", "NORMAL-001"},
	}
	for _, c := range cases {
		if got := CleanTitle(c.input); got != c.want {
			t.Errorf("CleanTitle(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestInfer(t *testing.T) {
	cases := []struct {
		name                            string
		sub, fourK, uncensored, cracked bool
	}{
		{"SSIS-001_FHD_CH", true, false, false, false},
		{"SSIS-001-UC", true, false, true, false},
		{"SSIS-001-C 中文字幕", true, false, false, false},
		{"SSIS-001 4K UHD", false, true, false, false},
		{"SSIS-001 2160p", false, true, false, false},
		{"SSIS-001_uncensored", false, false, true, false},
		{"SSIS-001 流出破解版", false, false, false, true},
		{"SSIS-001.Leaked.720p", false, false, false, true},
		{"(無修正-流出) SSIS-001 (Uncensored Leaked)一ヶ月間の禁欲の果てに", false, false, true, true},
		{"SSIS-001 禁欲生活 葵司 乙白", true, false, false, false}, // hanzi title without kana
	}
	for _, c := range cases {
		inf := Infer(c.name)
		if inf.HasSubtitle != c.sub || inf.Has4K != c.fourK || inf.HasUncensored != c.uncensored || inf.HasCracked != c.cracked {
			t.Errorf("%s: got %+v, want sub=%v 4k=%v uncensored=%v cracked=%v", c.name, inf, c.sub, c.fourK, c.uncensored, c.cracked)
		}
	}
}

func TestApplyInferenceKeepsSiteFlags(t *testing.T) {
	m := domain.Magnet{Name: "SSIS-001 4K 破解版 中文字幕", HD: true, Tags: []string{domain.MagnetTagHD}}
	ApplyInference(&m)
	if m.HasSubtitle || !m.HD {
		t.Error("inference must not rewrite site flags")
	}
	if !m.Inferred {
		t.Error("expected Inferred to be set when tags were added")
	}
	for _, tag := range []string{domain.MagnetTagHD, domain.MagnetTagSubtitle, domain.MagnetTag4K, domain.MagnetTagCracked} {
		if !slices.Contains(m.Tags, tag) {
			t.Errorf("expected tag %s in %v", tag, m.Tags)
		}
	}
	if !HasSubtitle(m) || !IsHD(m) || !IsUncensored(m) {
		t.Errorf("predicates should read inferred tags: %+v", m)
	}
	before := len(m.Tags)
	ApplyInference(&m)
	if len(m.Tags) != before {
		t.Errorf("second application must be a no-op, tags grew to %v", m.Tags)
	}
}

func TestApplyInferenceLeavesVerifiedMagnetUnmarked(t *testing.T) {
	m := domain.Magnet{Name: "SSIS-001-C", HasSubtitle: true, Tags: []string{domain.MagnetTagSubtitle}}
	ApplyInference(&m)
	if m.Inferred {
		t.Errorf("a site-labelled subtitle that the name also implies is not inferred: %+v", m)
	}
}
