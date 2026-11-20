package javdb

import (
	"slices"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/magnet"
)

func TestMagnetsDecodesUnitsAndPreservesSourceOrder(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v1/movies/movie-exact/magnets": fixtureFile(t, "magnets.json"),
	}}
	magnets, err := clientWithTransport(transport).Magnets(t.Context(), "movie-exact")
	if err != nil {
		t.Fatal(err)
	}
	if len(magnets) != 3 {
		t.Fatalf("magnet count = %d", len(magnets))
	}
	if magnets[0].Name != "Subtitle fixture" || magnets[1].Name != "HD fixture" || magnets[2].Name != "HD subtitle fixture" {
		t.Fatalf("unexpected source order: %s, %s, %s", magnets[0].Name, magnets[1].Name, magnets[2].Name)
	}
	if magnets[2].Size != 1024*1024*1024 || !magnets[2].HasSubtitle || !magnets[2].HD || magnets[2].FilesCount != 3 || magnets[2].CreatedAt != "2026-08-03" {
		t.Fatalf("magnet metadata = %#v", magnets[2])
	}
	for _, item := range magnets {
		if !slices.Equal(item.Sources, []string{domain.MagnetSourceJavDB}) ||
			item.HD != slices.Contains(item.Tags, domain.MagnetTagHD) ||
			item.HasSubtitle != slices.Contains(item.Tags, domain.MagnetTagSubtitle) {
			t.Errorf("source metadata disagrees with site flags: %+v", item)
		}
	}
}

func TestMagnetsNormalizeInfoHash(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v1/movies/movie/magnets": []byte(`{"success":1,"data":{"magnets":[{"hash":"ABCDEF0123456789ABCDEF0123456789ABCDEF01"}]}}`),
	}}
	magnets, err := clientWithTransport(transport).Magnets(t.Context(), "movie")
	if err != nil || len(magnets) != 1 || magnets[0].Hash != "abcdef0123456789abcdef0123456789abcdef01" {
		t.Fatalf("normalized magnets = %+v, error = %v", magnets, err)
	}
}

func TestFindWithoutJavDBIDDoesNotSearchByCode(t *testing.T) {
	transport := &stubTransport{}
	magnets, err := clientWithTransport(transport).Find(t.Context(), domain.MovieRef{JavDBID: " ", Code: "ABP-001"})
	if err != nil || len(magnets) != 0 || transport.calls != 0 {
		t.Fatalf("Find = %v, %v; requests = %d", magnets, err, transport.calls)
	}
}

func TestAggregatorRanksJavDBResources(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v1/movies/movie-exact/magnets": fixtureFile(t, "magnets.json"),
	}}
	aggregator := magnet.NewAggregator([]magnet.Source{clientWithTransport(transport)}, 0)
	magnets, err := aggregator.Find(t.Context(), domain.MovieRef{JavDBID: " movie-exact ", Code: "ABP-001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(magnets) != 3 {
		t.Fatalf("magnet count = %d", len(magnets))
	}
	if magnets[0].Name != "HD subtitle fixture" || magnets[1].Name != "Subtitle fixture" || magnets[2].Name != "HD fixture" {
		t.Fatalf("unexpected ranking: %s, %s, %s", magnets[0].Name, magnets[1].Name, magnets[2].Name)
	}
}

func TestMagnetsRejectsMalformedInfoHash(t *testing.T) {
	for _, hash := range []string{"", "abc", "not-a-hash"} {
		transport := &fixtureTransport{responses: map[string][]byte{
			"/api/v1/movies/movie/magnets": []byte(`{"success":1,"data":{"magnets":[{"hash":"` + hash + `"}]}}`),
		}}
		if _, err := clientWithTransport(transport).Magnets(t.Context(), "movie"); err == nil {
			t.Errorf("accepted invalid hash %q", hash)
		}
	}
}
