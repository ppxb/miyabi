package javdb

import "testing"

func TestMagnetsDecodesUnitsAndRanksResources(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v1/movies/movie-exact/magnets|zh-TW": fixtureFile(t, "magnets.json"),
	}}
	magnets, err := clientWithTransport(transport).Magnets(t.Context(), "movie-exact")
	if err != nil {
		t.Fatal(err)
	}
	if len(magnets) != 3 {
		t.Fatalf("magnet count = %d", len(magnets))
	}
	if magnets[0].Name != "HD subtitle fixture" || magnets[1].Name != "Subtitle fixture" || magnets[2].Name != "HD fixture" {
		t.Fatalf("unexpected ranking: %s, %s, %s", magnets[0].Name, magnets[1].Name, magnets[2].Name)
	}
	if magnets[0].Size != 1024*1024*1024 || !magnets[0].HasSubtitle || !magnets[0].HD || magnets[0].FilesCount != 3 || magnets[0].CreatedAt != "2026-08-03" {
		t.Fatalf("magnet metadata = %#v", magnets[0])
	}
}

func TestMagnetsRejectsMalformedInfoHash(t *testing.T) {
	for _, hash := range []string{"", "abc", "not-a-hash"} {
		transport := &fixtureTransport{responses: map[string][]byte{
			"/api/v1/movies/movie/magnets|zh-TW": []byte(`{"success":1,"data":{"magnets":[{"hash":"` + hash + `"}]}}`),
		}}
		if _, err := clientWithTransport(transport).Magnets(t.Context(), "movie"); err == nil {
			t.Errorf("accepted invalid hash %q", hash)
		}
	}
}
