package providers

import (
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
)

func fanzaSearchResult(ids ...string) any {
	contents := make([]any, len(ids))
	for i, id := range ids {
		contents[i] = map[string]any{"id": id}
	}
	return map[string]any{"data": map[string]any{"legacySearchPPV": map[string]any{"result": map[string]any{"contents": contents}}}}
}

func TestFANZAResolvesActressByCurrentOrFormerName(t *testing.T) {
	s := testFANZA(t, func(r fanzaRequest) any {
		if strings.Contains(r.Query, "query ActressSearch") {
			if r.Variables["keyword"] != "河北彩伽" || strings.Contains(r.Query, "actressIds") || !strings.Contains(r.Query, "sort: RELEASE_DATE") {
				t.Errorf("name search: %+v", r)
			}
			return fanzaSearchResult("p1")
		}
		return map[string]any{"data": map[string]any{"m0": map[string]any{"id": "p1", "makerContentId": "OFJE-682", "actresses": []any{
			map[string]any{"id": "1099472", "name": "瀬戸環奈"},
			map[string]any{"id": "1044864", "name": "河北彩花（河北彩伽）"},
		}}}}
	})
	id, err := s.ResolveActress(t.Context(), []string{"", "河北彩伽"})
	if err != nil || id != "1044864" {
		t.Fatalf("former name: id=%q err=%v", id, err)
	}
}

func TestFANZARejectsAmbiguousActressAndReportsNone(t *testing.T) {
	actresses := []any{map[string]any{"id": "1", "name": "同名"}}
	s := testFANZA(t, func(r fanzaRequest) any {
		if strings.Contains(r.Query, "query ActressSearch") {
			return fanzaSearchResult("p1", "p2")
		}
		second := []any{map[string]any{"id": "2", "name": "同名"}}
		return map[string]any{"data": map[string]any{
			"m0": map[string]any{"id": "p1", "makerContentId": "A-1", "actresses": actresses},
			"m1": map[string]any{"id": "p2", "makerContentId": "A-2", "actresses": second},
		}}
	})
	if _, err := s.ResolveActress(t.Context(), []string{"同名"}); !domain.IsKind(err, domain.KindConflict) {
		t.Fatalf("namesakes must not be guessed: %v", err)
	}
	if id, err := s.ResolveActress(t.Context(), []string{"別人"}); err != nil || id != "" {
		t.Fatalf("an unmatched name finds nobody: id=%q err=%v", id, err)
	}
}

func TestFANZAListsActressWorksNewestFirst(t *testing.T) {
	s := testFANZA(t, func(r fanzaRequest) any {
		if strings.Contains(r.Query, "query ActressSearch") {
			if r.Variables["actress"] != "1044864" || !strings.Contains(r.Query, "actressIds: { ids: [{ id: $actress }] }") || !strings.Contains(r.Query, "sort: RELEASE_DATE") {
				t.Errorf("works search: %+v", r)
			}
			return fanzaSearchResult("snos00535", "withdrawn")
		}
		return map[string]any{"data": map[string]any{
			"m0": map[string]any{"id": "snos00535", "makerContentId": "snos-535", "title": "Title",
				"makerReleasedAt": "2026-12-20T15:00:00Z", "deliveryStartDate": "2026-12-16T15:00:00Z",
				"packageImage": map[string]any{"largeUrl": "large.jpg", "mediumUrl": "medium.jpg"}},
			"m1": nil,
		}}
	})
	movies, err := s.ActressWorks(t.Context(), "1044864")
	if err != nil || len(movies) != 1 {
		t.Fatalf("works: %+v %v", movies, err)
	}
	if m := movies[0]; m.ID != "SNOS-535" || m.Code != "SNOS-535" || m.Title != "Title" || m.Cover != "large.jpg" || m.ReleaseDate != "2026-12-21" {
		t.Fatalf("mapped work: %+v", m)
	}
}
