package domain

import (
	"reflect"
	"testing"
)

func TestMergeSourceToggles(t *testing.T) {
	registered := []SourceToggle{{ID: "a", Enabled: true}, {ID: "b", Enabled: true}, {ID: "c", Enabled: true}}
	saved := []SourceToggle{{ID: "b", Enabled: false}, {ID: "removed", Enabled: false}, {ID: "a", Enabled: true}, {ID: "b", Enabled: true}}

	got := MergeSourceToggles(saved, registered)
	want := []SourceToggle{{ID: "b", Enabled: false}, {ID: "a", Enabled: true}, {ID: "c", Enabled: true}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merge = %+v, want %+v", got, want)
	}
}

func TestValidateSourceToggles(t *testing.T) {
	known := map[string]bool{"a": true, "b": true}
	if err := ValidateSourceToggles([]SourceToggle{{ID: "b"}, {ID: "a"}}, known, "来源"); err != nil {
		t.Fatalf("valid list rejected: %v", err)
	}
	for _, settings := range [][]SourceToggle{
		{{ID: "a"}},
		{{ID: "a"}, {ID: "a"}},
		{{ID: "a"}, {ID: "x"}},
	} {
		if err := ValidateSourceToggles(settings, known, "来源"); !IsKind(err, KindInvalid) {
			t.Fatalf("settings %+v: err = %v, want invalid", settings, err)
		}
	}
}
