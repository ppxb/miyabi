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
	resolveCalls     int
	resolvedID       string
	resolveErr       error
	err              error
	searchByQuery    map[string][]catalogue.Movie
	queried          []string
	requestedID      string
}

func (s *javdbStub) ResolveMovieID(_ context.Context, code string) (string, error) {
	s.resolveCalls++
	s.queried = append(s.queried, code)
	if s.resolveErr != nil {
		return "", s.resolveErr
	}
	if s.resolvedID == "" {
		return "", metadata.ErrNotFound
	}
	return s.resolvedID, nil
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
	s := &javdbStub{resolvedID: "known", detail: domain.MovieDetail{Movie: domain.Movie{ID: "known", Code: "ABP-123", Title: "Title", Cover: "cover",
		Actors: []domain.Actor{{ID: "actor", Name: "Actor"}}, Tags: []domain.Tag{{ID: "tag", Name: "Tag"}}, Maker: &domain.Maker{ID: "maker", Name: "Studio"}}}}
	m, err := NewJavDB(s).Fetch(t.Context(), domain.MovieRef{Code: "ABP-123"})
	if err != nil || s.resolveCalls != 1 || s.details != 1 || m.Detail.Sources[0].Provider != "javdb" || m.Detail.Actors[0].Provider != "javdb" || m.Images[0].Provider != "javdb" {
		t.Fatalf("result=%+v err=%v", m, err)
	}
	if s.detail.Actors[0].Provider != "" || s.detail.Tags[0].Provider != "" || s.detail.Maker.Provider != "" {
		t.Fatal("catalogue cache mutated")
	}
}

func TestJavDBRefreshRequestsFreshCatalogueDetail(t *testing.T) {
	s := &javdbStub{resolvedID: "known", detail: domain.MovieDetail{Movie: domain.Movie{ID: "known", Code: "IPZZ-960", Title: "JavDB title"}}}
	m, err := NewJavDB(s).Fetch(t.Context(), domain.MovieRef{Code: "IPZZ-960", Refresh: true})
	if err != nil || s.refreshes != 1 || s.resolveCalls != 1 || m.Detail.Title != "JavDB title" {
		t.Fatalf("JavDB refresh skipped: %+v %v", m, err)
	}
}

func TestJavDBResolvesByCodeWithoutKnownID(t *testing.T) {
	s := &javdbStub{resolvedID: "known", detail: domain.MovieDetail{Movie: domain.Movie{ID: "known", Code: "ABP-123", Title: "Title"}}}
	m, err := NewJavDB(s).Fetch(t.Context(), domain.MovieRef{Code: "ABP-123"})
	if err != nil || m.Detail.ID != "known" || s.resolveCalls != 1 || s.details != 1 {
		t.Fatalf("code resolution failed: result=%+v err=%v", m, err)
	}
}

func TestJavDBDetailCannotOverrideConfirmedNumber(t *testing.T) {
	s := &javdbStub{detail: domain.MovieDetail{Movie: domain.Movie{ID: "known", Code: "ABP-124", Title: "Wrong movie"}}}
	_, err := NewJavDB(s).Fetch(t.Context(), domain.MovieRef{Code: "ABP-123"})
	if !errors.Is(err, metadata.ErrNotFound) {
		t.Fatal("wrong movie accepted")
	}
}

func TestJavDBPropagatesResolveError(t *testing.T) {
	s := &javdbStub{resolveErr: domain.E(domain.KindConflict, "ambiguous", nil)}
	_, err := NewJavDB(s).Fetch(t.Context(), domain.MovieRef{Code: "ABP-123"})
	if domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("resolve error not propagated: %v", err)
	}
}

func TestJavDBRejectsAmbiguousYearVariants(t *testing.T) {
	s := &javdbStub{resolveErr: domain.E(domain.KindConflict, "ambiguous year spelling", nil)}
	_, err := NewJavDB(s).Fetch(t.Context(), domain.MovieRef{Code: "STUDIO.26.09.27"})
	if domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("ambiguity not reported: %v", err)
	}
}
