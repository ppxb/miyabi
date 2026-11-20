package providers

import (
	"context"
	"fmt"
	"slices"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
)

// The catalogue owns its clients and cache; this adapter does not close them.
type JavDBCatalogue interface {
	CatalogueDetail(context.Context, string) (domain.MovieDetail, error)
	RefreshCatalogueDetail(context.Context, string) (domain.MovieDetail, error)
	ResolveMovieID(context.Context, string) (string, error)
	Media(context.Context, string) (domain.Media, error)
}

type JavDB struct{ catalogue JavDBCatalogue }

func NewJavDB(catalogue JavDBCatalogue) *JavDB { return &JavDB{catalogue: catalogue} }
func (*JavDB) ID() string                      { return "javdb" }
func (*JavDB) Supports(string) bool            { return true }
func (s *JavDB) Media(ctx context.Context, url string) (domain.Media, error) {
	return s.catalogue.Media(ctx, url)
}

func (s *JavDB) Fetch(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	id, err := s.catalogue.ResolveMovieID(ctx, code)
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	detail := s.catalogue.CatalogueDetail
	if ref.Refresh {
		detail = s.catalogue.RefreshCatalogueDetail
	}
	m, err := detail(ctx, id)
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	if !codeid.IsFormatEquivalent(m.Code, code) {
		return domain.MovieMetadata{}, metadata.ErrNotFound
	}
	if m.ID != id || m.Title == "" {
		return domain.MovieMetadata{}, fmt.Errorf("JavDB 返回了不完整的影片身份")
	}
	m.Sources = []domain.SourceID{{Provider: "javdb", ID: id}}
	m.RatingSource, m.RatingMax = "javdb", 5
	// Never mutate the catalogue's shared detail cache while assigning source identities.
	m.Actors = slices.Clone(m.Actors)
	for i := range m.Actors {
		m.Actors[i].Provider = "javdb"
	}
	m.Tags = slices.Clone(m.Tags)
	for i := range m.Tags {
		m.Tags[i].Provider = "javdb"
	}
	if m.Maker != nil {
		copy := *m.Maker
		copy.Provider = "javdb"
		m.Maker = &copy
	}
	if m.Series != nil {
		copy := *m.Series
		copy.Provider = "javdb"
		m.Series = &copy
	}
	if m.Director != nil {
		copy := *m.Director
		copy.Provider = "javdb"
		m.Director = &copy
	}
	result := domain.MovieMetadata{Detail: m}
	if m.Cover != "" {
		result.Images = append(result.Images, domain.ImageCandidate{Provider: "javdb", URL: m.Cover, Role: "cover"})
	}
	for _, image := range m.PreviewImages {
		url := image.Original
		if url == "" {
			url = image.Thumbnail
		}
		if url != "" {
			result.Images = append(result.Images, domain.ImageCandidate{Provider: "javdb", URL: url, Role: "preview"})
		}
	}
	return result, nil
}
