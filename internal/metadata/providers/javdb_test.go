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
	search           []catalogue.Movie
	detail           domain.MovieDetail
	queries, details int
	err              error
}

func (s *javdbStub) Search(context.Context, string, domain.SearchOptions) ([]catalogue.Movie, error) {
	s.queries++
	return s.search, s.err
}
func (s *javdbStub) CatalogueDetail(context.Context, string) (domain.MovieDetail, error) {
	s.details++
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
