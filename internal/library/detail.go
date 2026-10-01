package library

import (
	"context"
	"fmt"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/nfo"
)

type MovieDetail struct {
	domain.MovieDetail
	LibraryID    int                `json:"library_id"`
	ScrapeStatus movie.ScrapeStatus `json:"scrape_status"`
}

// Movie reads saved metadata within the same scope as the library list. Opening
// a detail never resolves a catalogue identity or starts a scraping task.
func (s *Service) Movie(ctx context.Context, id int) (MovieDetail, error) {
	record, err := s.detailRecord(ctx, id)
	if err != nil {
		return MovieDetail{}, err
	}
	doc := domain.ValueOrZero(record.Metadata)
	detail := MovieDetail{LibraryID: record.ID, ScrapeStatus: record.ScrapeStatus}
	detail.MovieDetail = domain.MovieDetail{
		Zone: doc.Zone,
		Movie: domain.Movie{
			ID: doc.JavDBID(), Code: record.Code, Title: doc.Title, Summary: doc.Summary,
			ReleaseDate: doc.Premiered, Duration: doc.Runtime, Rating: doc.Rating,
			RatingSource: doc.RatingSource, RatingMax: doc.RatingMax, FieldSources: doc.FieldSources,
			Thumbnail: domain.ValueOrZero(record.Cover), Cover: domain.ValueOrZero(record.Poster),
			Actors: []domain.Actor{}, Tags: []domain.Tag{}, PreviewImages: []domain.PreviewImage{},
		},
		ActorMovies: []domain.MovieReference{}, RelatedMovies: []domain.MovieReference{},
	}
	if len(record.Fanarts) > 0 {
		detail.Cover = record.Fanarts[0]
	}
	if detail.Zone == "" {
		detail.Zone = domain.ZoneUnknown
	}
	for _, id := range doc.IDs {
		detail.Sources = append(detail.Sources, domain.SourceID{Provider: id.Type, ID: id.Value})
	}
	for _, person := range doc.Actors {
		detail.Actors = append(detail.Actors, domain.Actor{Provider: person.Provider, ID: person.ID,
			Name: person.Name, NameZHT: person.NameZHT, Gender: person.Gender})
	}
	for _, tag := range doc.Tags {
		detail.Tags = append(detail.Tags, domain.Tag{Provider: tag.Provider, ID: tag.ID,
			Name: tag.Name, NameZHT: tag.NameZHT, CategoryID: tag.CategoryID})
	}
	if doc.Studio.Name != "" {
		detail.Maker = &domain.Maker{Provider: doc.Studio.Provider, ID: doc.Studio.ID, Name: doc.Studio.Name}
	}
	if doc.Set.Name != "" {
		detail.Series = &domain.Series{Provider: doc.Set.Provider, ID: doc.Set.ID, Name: doc.Set.Name}
	}
	if doc.Director.Name != "" {
		detail.Director = &domain.Director{Provider: doc.Director.Provider, ID: doc.Director.ID, Name: doc.Director.Name}
	}
	for i := range previewCandidates(doc) {
		url := fmt.Sprintf("/api/library/movies/%d/previews/%d?v=%d", record.ID, i, record.UpdatedAt.UnixMilli())
		detail.PreviewImages = append(detail.PreviewImages, domain.PreviewImage{Original: url, Thumbnail: url})
	}
	detail.HasPreview = len(detail.PreviewImages) > 0
	return detail, nil
}

func (s *Service) Preview(ctx context.Context, id, index int) (domain.ImageCandidate, error) {
	record, err := s.detailRecord(ctx, id)
	if err != nil {
		return domain.ImageCandidate{}, err
	}
	candidates := previewCandidates(domain.ValueOrZero(record.Metadata))
	if index < 0 || index >= len(candidates) {
		return domain.ImageCandidate{}, domain.E(domain.KindNotFound, "预览图不存在", nil)
	}
	return candidates[index], nil
}

func (s *Service) detailRecord(ctx context.Context, id int) (*ent.Movie, error) {
	record, err := s.database.Movie.Query().Where(movie.IDEQ(id), movie.HasFilesWith(libraryScope(s.Source()))).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, domain.E(domain.KindNotFound, "影片不在当前媒体库中", err)
	}
	return record, err
}

func previewCandidates(doc nfo.Movie) []domain.ImageCandidate {
	var images []domain.ImageCandidate
	for _, candidate := range doc.Images {
		if candidate.Role == "preview" && candidate.Provider != "" && candidate.URL != "" {
			images = append(images, candidate)
		}
	}
	return images
}
