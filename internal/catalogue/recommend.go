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
// already indexed locally, so opening a detail never triggers extra scraping:
// "also appeared" comes from shared actors, "you may like" from shared series,
// maker or tags.
func (service *Service) LocalRecommendations(ctx context.Context, code string) (actorMovies, relatedMovies []domain.MovieReference, found bool, err error) {
	actorMovies, relatedMovies = []domain.MovieReference{}, []domain.MovieReference{}
	if service.database == nil {
		return actorMovies, relatedMovies, false, nil
	}
	code = codeid.Normalize(code)
	current, err := service.database.Movie.Query().
		Where(movie.CanonicalCodeEQ(codeid.MatchKey(code))).
		Order(ent.Asc(movie.FieldID)).
		WithActors().WithTags().
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return actorMovies, relatedMovies, false, nil
		}
		return actorMovies, relatedMovies, false, err
	}

	exclude := movie.IDNEQ(current.ID)
	seen := map[string]bool{current.Code: true}

	actorIDs := make([]int, 0, len(current.Edges.Actors))
	for _, person := range current.Edges.Actors {
		actorIDs = append(actorIDs, person.ID)
	}
	if len(actorIDs) > 0 {
		records, queryErr := service.database.Movie.Query().
			Where(exclude, movie.HasActorsWith(actor.IDIn(actorIDs...))).
			Order(ent.Desc(movie.FieldUpdatedAt)).
			Limit(recommendationLimit).
			Select(movie.FieldCode, movie.FieldCover, movie.FieldPoster).
			All(ctx)
		if queryErr != nil {
			return actorMovies, relatedMovies, true, queryErr
		}
		for _, record := range records {
			if seen[record.Code] {
				continue
			}
			seen[record.Code] = true
			actorMovies = append(actorMovies, localReference(record))
		}
	}

	preds := make([]predicate.Movie, 0, 3)
	if current.SeriesID != nil && *current.SeriesID != "" {
		preds = append(preds, movie.SeriesIDEQ(*current.SeriesID))
	}
	if current.MakerID != nil && *current.MakerID != "" {
		preds = append(preds, movie.MakerIDEQ(*current.MakerID))
	}
	tagIDs := make([]int, 0, len(current.Edges.Tags))
	for _, item := range current.Edges.Tags {
		tagIDs = append(tagIDs, item.ID)
	}
	if len(tagIDs) > 0 {
		preds = append(preds, movie.HasTagsWith(tag.IDIn(tagIDs...)))
	}
	if len(preds) > 0 {
		records, queryErr := service.database.Movie.Query().
			Where(exclude, movie.Or(preds...)).
			Order(ent.Desc(movie.FieldUpdatedAt)).
			Limit(recommendationLimit).
			Select(movie.FieldCode, movie.FieldCover, movie.FieldPoster).
			All(ctx)
		if queryErr != nil {
			return actorMovies, relatedMovies, true, queryErr
		}
		for _, record := range records {
			if seen[record.Code] {
				continue
			}
			seen[record.Code] = true
			relatedMovies = append(relatedMovies, localReference(record))
		}
	}
	return actorMovies, relatedMovies, true, nil
}

func localReference(record *ent.Movie) domain.MovieReference {
	thumbnail := domain.ValueOrZero(record.Poster)
	if thumbnail == "" {
		thumbnail = domain.ValueOrZero(record.Cover)
	}
	return domain.MovieReference{
		ID:        codeid.Normalize(record.Code),
		Code:      record.Code,
		Thumbnail: thumbnail,
	}
}
