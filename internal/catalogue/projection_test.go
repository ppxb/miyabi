package catalogue

import (
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
)

type unexpectedLocalState struct {
	LocalState
	t *testing.T
}

func (s unexpectedLocalState) Source() *domain.LibrarySource {
	s.t.Fatal("catalogue metadata requested local state")
	return nil
}

func TestCatalogueMetadataDoesNotReadLocalState(t *testing.T) {
	provider := &stubProviderWithMagnets{movies: []domain.Movie{{ID: "movie-1", Code: "SSIS-001"}}}
	service := newService(nil, provider, nil, unexpectedLocalState{t: t}, persistedRoute{})
	defer service.Close()
	if movies, err := service.Search(t.Context(), "SSIS-001", domain.SearchOptions{}); err != nil || len(movies) != 1 {
		t.Fatalf("search = %+v, %v", movies, err)
	}
	if movies, err := service.Browse(t.Context(), domain.BrowseOptions{}); err != nil || len(movies) != 1 {
		t.Fatalf("browse = %+v, %v", movies, err)
	}
	if detail, err := service.MovieDetail(t.Context(), "movie-1"); err != nil || detail.ID != "movie-1" {
		t.Fatalf("detail = %+v, %v", detail, err)
	}
}
