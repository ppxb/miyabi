package providers

import (
	"context"
	"errors"
	"testing"

	"github.com/ppxb/miyabi/internal/catalogue"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
)

type javdbStub struct {
	refreshes        int
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

func (s *javdbStub) RefreshCatalogueDetail(ctx context.Context, id string) (domain.MovieDetail, error) {
	s.refreshes++
	return s.CatalogueDetail(ctx, id)
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

func TestJavDBRefreshRequestsFreshCatalogueDetail(t *testing.T) {
	s := &javdbStub{detail: domain.MovieDetail{Movie: domain.Movie{ID: "known", Code: "IPZZ-960", Title: "JavDB title"}}}
	m, err := NewJavDB(s).Fetch(t.Context(), domain.MovieRef{Code: "IPZZ-960", JavDBID: "known", Refresh: true})
	if err != nil || s.refreshes != 1 || s.queries != 0 || m.Detail.Title != "JavDB title" {
		t.Fatalf("JavDB refresh skipped: %+v %v", m, err)
	}
}

func TestJavDBRequiresAKnownID(t *testing.T) {
	s := &javdbStub{search: []catalogue.Movie{{Movie: domain.Movie{ID: "weaker", Code: "ABP-123"}}}}
	_, err := NewJavDB(s).Fetch(t.Context(), domain.MovieRef{Code: "ABP-123"})
	if !errors.Is(err, metadata.ErrNotFound) || s.details != 0 || s.queries != 0 {
		t.Fatalf("id-less lookup must not search: %v", err)
	}
}

func TestJavDBDetailCannotOverrideConfirmedNumber(t *testing.T) {
	s := &javdbStub{detail: domain.MovieDetail{Movie: domain.Movie{ID: "known", Code: "ABP-124", Title: "Wrong movie"}}}
	_, err := NewJavDB(s).Fetch(t.Context(), domain.MovieRef{Code: "ABP-123", JavDBID: "known"})
	if !errors.Is(err, metadata.ErrNotFound) {
		t.Fatal("wrong movie accepted")
	}
}

func TestJavDBDoesNotSearchWithoutKnownID(t *testing.T) {
	s := &javdbStub{}
	_, err := NewJavDB(s).Fetch(t.Context(), domain.MovieRef{Code: "TUSHYRAW.26.09.27"})
	if !errors.Is(err, metadata.ErrNotFound) || s.queries != 0 || s.details != 0 {
		t.Fatalf("expected not found without search: %v", err)
	}
}

func TestJavDBRejectsAmbiguousYearVariants(t *testing.T) {
	s := &javdbStub{search: []catalogue.Movie{
		{Movie: domain.Movie{ID: "one", Code: "STUDIO.2026.09.27"}},
		{Movie: domain.Movie{ID: "two", Code: "STUDIO.2026.09.27"}},
	}}
	_, err := NewJavDB(s).Fetch(t.Context(), domain.MovieRef{Code: "STUDIO.26.09.27"})
	if !errors.Is(err, metadata.ErrNotFound) || s.queries != 0 {
		t.Fatalf("id-less lookup must not search: %v", err)
	}
}
