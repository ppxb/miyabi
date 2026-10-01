package providers

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/ppxb/miyabi/internal/catalogue"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
)

type javdbStub struct {
	search           []catalogue.Movie
	detail           domain.MovieDetail
	queries, details int
	err              error
	searchByQuery    map[string][]catalogue.Movie
	queried          []string
	requestedID      string
}

func (s *javdbStub) Search(_ context.Context, query string, _ domain.SearchOptions) ([]catalogue.Movie, error) {
	s.queries++
	s.queried = append(s.queried, query)
	if s.searchByQuery != nil {
		return s.searchByQuery[query], s.err
	}
	return s.search, s.err
}
func (s *javdbStub) CatalogueDetail(_ context.Context, id string) (domain.MovieDetail, error) {
	s.details++
	s.requestedID = id
	return s.detail, s.err
}
func (*javdbStub) Media(context.Context, string) (domain.Media, error) { return domain.Media{}, nil }

func TestJavDBKnownIDSkipsSearchAndDoesNotMutateCatalogueCache(t *testing.T) {
	s := &javdbStub{detail: domain.MovieDetail{Movie: domain.Movie{ID: "known", Code: "ABP-123", Title: "Title", Cover: "cover",
		Actors: []domain.Actor{{ID: "actor", Name: "Actor"}}, Tags: []domain.Tag{{ID: "tag", Name: "Tag"}}, Maker: &domain.Maker{ID: "maker", Name: "Studio"}}}}
	m, err := NewJavDB(s).Fetch(t.Context(), domain.MovieRef{Code: "ABP-123", JavDBID: "known"})
	if err != nil || s.queries != 0 || s.details != 1 || m.Detail.Sources[0].Provider != "javdb" || m.Detail.Actors[0].Provider != "javdb" || m.Images[0].Provider != "javdb" {
		t.Fatalf("result=%+v err=%v", m, err)
	}
	if s.detail.Actors[0].Provider != "" || s.detail.Tags[0].Provider != "" || s.detail.Maker.Provider != "" {
		t.Fatal("catalogue cache mutated")
	}
}

func TestJavDBFallbackDoesNotPerformItsOwnWeakerMatching(t *testing.T) {
	s := &javdbStub{search: []catalogue.Movie{{Movie: domain.Movie{ID: "weaker", Code: "ABP-123"}}}}
	_, err := NewJavDB(s).Fetch(t.Context(), domain.MovieRef{Code: "118ABP-123"})
	if !errors.Is(err, metadata.ErrNotFound) || s.details != 0 {
		t.Fatalf("weaker match accepted: %v", err)
	}
	s.search = append(s.search, catalogue.Movie{Movie: domain.Movie{ID: "other", Code: "ABP-123"}})
	_, err = NewJavDB(s).Fetch(t.Context(), domain.MovieRef{Code: "ABP-123"})
	if domain.KindOf(err) != domain.KindConflict || s.details != 0 {
		t.Fatalf("ambiguous match accepted: %v", err)
	}
}

func TestJavDBDetailCannotOverrideConfirmedNumber(t *testing.T) {
	s := &javdbStub{detail: domain.MovieDetail{Movie: domain.Movie{ID: "known", Code: "ABP-124", Title: "Wrong movie"}}}
	_, err := NewJavDB(s).Fetch(t.Context(), domain.MovieRef{Code: "ABP-123", JavDBID: "known"})
	if !errors.Is(err, metadata.ErrNotFound) {
		t.Fatal("wrong movie accepted")
	}
}

func TestJavDBResolvesFilenameCodesWithoutKnownID(t *testing.T) {
	for _, tt := range []struct {
		code, canonical, id, query, other string
		queries                           []string
	}{
		{
			code: "PACOPACOMAMA-042126-100", canonical: "042126_100", id: "AqVqkn",
			query: "042126-100", other: "042126_101",
			queries: []string{"PACOPACOMAMA-042126-100", "042126-100", "042126_100"},
		},
		{
			code: "TUSHYRAW.26.09.27", canonical: "Tushyraw.2026.09.27", id: "XeppbV",
			query: "TUSHYRAW.2026.09.27", other: "Tushyraw.2023.09.27",
			queries: []string{"TUSHYRAW.26.09.27", "TUSHYRAW.2026.09.27"},
		},
	} {
		t.Run(tt.code, func(t *testing.T) {
			catalogue := &javdbStub{
				searchByQuery: map[string][]catalogue.Movie{tt.query: {
					{Movie: domain.Movie{ID: "unrelated", Code: tt.other}},
					{Movie: domain.Movie{ID: tt.id, Code: tt.canonical}},
				}},
				detail: domain.MovieDetail{Movie: domain.Movie{ID: tt.id, Code: tt.canonical, Title: "Confirmed film"}},
			}
			store, err := database.Open(t.Context(), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			service, err := metadata.New(t.Context(), store.Client, NewJavDB(catalogue))
			if err != nil {
				t.Fatal(err)
			}
			defer service.Close()
			for range 2 {
				m, err := service.Resolve(t.Context(), domain.MovieRef{Code: tt.code})
				if err != nil || m.Detail.ID != tt.id || m.Detail.Code != tt.canonical || catalogue.requestedID != tt.id {
					t.Fatalf("filename lookup failed: %+v, requested=%s, err=%v", m, catalogue.requestedID, err)
				}
			}
			if !slices.Equal(catalogue.queried, tt.queries) || catalogue.details != 1 {
				t.Fatalf("unexpected lookup order or cache miss: %v, details=%d", catalogue.queried, catalogue.details)
			}
		})
	}
}

func TestJavDBRejectsAmbiguousYearVariants(t *testing.T) {
	s := &javdbStub{search: []catalogue.Movie{
		{Movie: domain.Movie{ID: "one", Code: "STUDIO.2026.09.27"}},
		{Movie: domain.Movie{ID: "two", Code: "STUDIO.2026.09.27"}},
	}}
	_, err := NewJavDB(s).Fetch(t.Context(), domain.MovieRef{Code: "STUDIO.26.09.27"})
	if domain.KindOf(err) != domain.KindConflict || s.details != 0 {
		t.Fatalf("ambiguous year spelling accepted: %v", err)
	}
}
