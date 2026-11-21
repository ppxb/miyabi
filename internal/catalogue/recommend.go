package catalogue

import (
	"context"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/predicate"
	"github.com/ppxb/miyabi/internal/ent/tag"
)

const recommendationLimit = 12

// LocalRecommendations derives the detail-page recommendation lists from data
// already indexed locally. It matches the movie's own metadata (actors, series,
// maker, tags) against the local library, so no extra catalogue scraping is
// needed and only locally-known titles are ever recommended.
func (service *Service) LocalRecommendations(ctx context.Context, meta domain.MovieMetadata) (actorMovies, relatedMovies []domain.MovieReference, err error) {
	actorMovies, relatedMovies = []domain.MovieReference{}, []domain.MovieReference{}
	detail := meta.Detail
	excludeCode := codeid.Normalize(detail.Code)
	seen := map[string]bool{excludeCode: true}

	var base []predicate.Movie
	if excludeCode != "" {
		base = append(base, movie.CodeNEQ(excludeCode))
	}

	if actorPred := actorPredicate(detail.Actors); actorPred != nil {
		records, queryErr := service.database.Movie.Query().
			Where(append(base, movie.HasActorsWith(actorPred))...).
			Order(ent.Desc(movie.FieldUpdatedAt)).
			Limit(recommendationLimit).
			Select(movie.FieldCode, movie.FieldCover, movie.FieldPoster, movie.FieldTitle, movie.FieldReleaseDate, movie.FieldRating).
			All(ctx)
		if queryErr != nil {
			return actorMovies, relatedMovies, queryErr
		}
		for _, record := range records {
			if seen[record.Code] {
				continue
			}
			seen[record.Code] = true
			actorMovies = append(actorMovies, localReference(record))
		}
	}

	related := make([]predicate.Movie, 0, 6)
	if series := detail.Series; series != nil {
		if series.ID != "" {
			related = append(related, movie.SeriesIDEQ(series.ID))
		}
		if series.Name != "" {
			related = append(related, movie.SeriesNameEQ(series.Name))
		}
	}
	if maker := detail.Maker; maker != nil {
		if maker.ID != "" {
			related = append(related, movie.MakerIDEQ(maker.ID))
		}
		if maker.Name != "" {
			related = append(related, movie.MakerNameEQ(maker.Name))
		}
	}
	if tagPred := tagPredicate(detail.Tags); tagPred != nil {
		related = append(related, movie.HasTagsWith(tagPred))
	}
	if len(related) > 0 {
		records, queryErr := service.database.Movie.Query().
			Where(append(base, movie.Or(related...))...).
			Order(ent.Desc(movie.FieldUpdatedAt)).
			Limit(recommendationLimit).
			Select(movie.FieldCode, movie.FieldCover, movie.FieldPoster, movie.FieldTitle, movie.FieldReleaseDate, movie.FieldRating).
			All(ctx)
		if queryErr != nil {
			return actorMovies, relatedMovies, queryErr
		}
		for _, record := range records {
			if seen[record.Code] {
				continue
			}
			seen[record.Code] = true
			relatedMovies = append(relatedMovies, localReference(record))
		}
	}
	return actorMovies, relatedMovies, nil
}

func actorPredicate(actors []domain.Actor) predicate.Actor {
	preds := make([]predicate.Actor, 0, len(actors)*2)
	for _, person := range actors {
		if person.Provider != "" && person.ID != "" {
			preds = append(preds, actor.And(actor.ProviderEQ(person.Provider), actor.SourceIDEQ(person.ID)))
		}
		if person.Name != "" {
			preds = append(preds, actor.NameEQ(person.Name))
		}
	}
	if len(preds) == 0 {
		return nil
	}
	return actor.Or(preds...)
}

func tagPredicate(tags []domain.Tag) predicate.Tag {
	preds := make([]predicate.Tag, 0, len(tags)*2)
	for _, item := range tags {
		if item.Provider != "" && item.ID != "" {
			preds = append(preds, tag.And(tag.ProviderEQ(item.Provider), tag.SourceIDEQ(item.ID)))
		}
		if item.Name != "" {
			preds = append(preds, tag.NameEQ(item.Name))
		}
	}
	if len(preds) == 0 {
		return nil
	}
	return tag.Or(preds...)
}

func localReference(record *ent.Movie) domain.MovieReference {
	thumbnail := domain.ValueOrZero(record.Poster)
	if thumbnail == "" {
		thumbnail = domain.ValueOrZero(record.Cover)
	}
	reference := domain.MovieReference{
		ID:        codeid.Normalize(record.Code),
		Code:      record.Code,
		Thumbnail: thumbnail,
		Title:     record.Title,
	}
	if record.ReleaseDate != nil {
		reference.ReleaseDate = record.ReleaseDate.Format("2006-01-02")
	}
	if record.Rating != nil {
		reference.Rating = *record.Rating
	}
	return reference
}
