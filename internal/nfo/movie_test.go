package nfo

import (
	"net/url"
	"strings"
	"testing"
)

func TestUnfamiliarNumbersRoundTripWithSafeFilenames(t *testing.T) {
	seen := make(map[string]bool)
	for _, code := range []string{"GLOD-0436", "KNB-M014", "Studio.26.09.05", "作品/限定 #007", "A/B", "A%2FB", "A+B", "A B", `A\B:001`} {
		stem := FileStem(code)
		if strings.ContainsAny(stem, `/\:*?"<>|`) || seen[stem] {
			t.Fatalf("unsafe or colliding filename for %q: %q", code, stem)
		}
		seen[stem] = true
		if decoded, err := url.QueryUnescape(stem); err != nil || decoded != code {
			t.Fatalf("filename lost catalogue identity: %q -> %q", code, stem)
		}
		doc := Movie{Code: code, Title: "Fixture", Thumbs: []Thumb{{Aspect: "poster", Path: stem + "-poster.jpg"}}}
		body, err := Encode(doc)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := Decode(body)
		if err != nil || restored.Code != code || restored.Poster() != doc.Poster() {
			t.Fatalf("number or artwork reference changed during NFO round trip: %#v, %v", restored, err)
		}
	}
	if FileStem("KNB-M014") != "KNB-M014" {
		t.Fatal("changed a conventional sidecar filename")
	}
}

func TestRoundTripPreservesSourceIDsAndOmitsPlot(t *testing.T) {
	input := []byte(`<movie>
  <title>Fixture &amp; title</title><num>ABP-001</num>
  <uniqueid type="javdb" default="true">fixture-movie</uniqueid>
  <premiered>2024-01-02</premiered><runtime>120</runtime><rating>4.5</rating>
  <director javdbid="director-1">Director</director>
  <studio javdbid="maker-1">Maker</studio><set javdbid="series-1"><name>Series</name></set>
  <actor><name>Actor</name><javdbid>actor-1</javdbid><name_zht>演員</name_zht><gender>female</gender></actor>
  <tag javdbid="tag-1" category="category-1" name_zht="標籤">Tag</tag>
  <thumb aspect="poster">ABP-001-poster.jpg</thumb><fanart><thumb>ABP-001-fanart.jpg</thumb></fanart>
  <plot>Must not be retained</plot>
</movie>`)
	doc, err := Decode(input)
	if err != nil {
		t.Fatal(err)
	}
	body, err := Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "plot") || strings.Contains(string(body), "Must not be retained") {
		t.Fatal("NFO retained plot content")
	}
	got, err := Decode(body)
	if err != nil {
		t.Fatal(err)
	}
	if got.Code != "ABP-001" || got.JavDBID() != "fixture-movie" || got.Title != "Fixture & title" ||
		got.Director.ID != "director-1" || got.Studio.ID != "maker-1" || got.Set.ID != "series-1" ||
		got.Runtime != 120 || got.Premiered != "2024-01-02" || got.Rating != 4.5 {
		t.Fatalf("movie metadata changed: %#v", got)
	}
	if len(got.Actors) != 1 || got.Actors[0].ID != "actor-1" || got.Actors[0].Gender != "female" || got.Actors[0].NameZHT != "演員" {
		t.Fatalf("actor metadata changed: %#v", got.Actors)
	}
	if len(got.Tags) != 1 || got.Tags[0].ID != "tag-1" || got.Tags[0].CategoryID != "category-1" {
		t.Fatalf("tag metadata changed: %#v", got.Tags)
	}
	if got.Poster() != "ABP-001-poster.jpg" || got.Fanart != "ABP-001-fanart.jpg" {
		t.Fatalf("relative artwork references changed: %#v", got)
	}
}
