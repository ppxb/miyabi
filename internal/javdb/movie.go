package javdb

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
)

// MovieDetail fetches a movie and its graph metadata.
func (c *Client) MovieDetail(ctx context.Context, movieID string) (domain.MovieDetail, error) {
	movieID = strings.TrimSpace(movieID)
	if movieID == "" {
		return domain.MovieDetail{}, errors.New("JavDB movie ID is required")
	}

	var data wireMovieData
	if c.webFallback.Load() {
		return c.movieDetailFromWeb(ctx, movieID)
	}
	if err := c.getJSON(
		ctx,
		"/api/v4/movies/"+url.PathEscape(movieID),
		nil,
		&data,
	); err != nil {
		if requiresWebFallback(err) {
			c.webFallback.Store(true)
			return c.movieDetailFromWeb(ctx, movieID)
		}
		return domain.MovieDetail{}, err
	}
	movie, err := movieFromWire(ctx, data.Movie.wireMovie)
	if err != nil {
		return domain.MovieDetail{}, err
	}
	zone := domain.ZoneUnknown
	if data.Movie.Type != nil {
		zone = zoneFromCode(*data.Movie.Type)
		if zone == domain.ZoneUnknown {
			slog.WarnContext(ctx, "unknown JavDB movie type; using unknown zone",
				"movie_id", movie.ID, "field", "type", "value", *data.Movie.Type)
		}
	}
	return domain.MovieDetail{
		Movie: movie, Zone: zone,
		ActorMovies:   movieReferencesFromWire(ctx, movie.ID, "actor_movies", data.Movie.ActorMovies),
		RelatedMovies: movieReferencesFromWire(ctx, movie.ID, "relative_movies", data.Movie.RelatedMovies),
	}, nil
}

func zoneFromCode(code int) domain.Zone {
	for zone, value := range zoneCodes {
		if value == code {
			return zone
		}
	}
	return domain.ZoneUnknown
}

func movieReferencesFromWire(ctx context.Context, movieID, field string, source []wireMovieReference) []domain.MovieReference {
	result := make([]domain.MovieReference, 0, len(source))
	for index, item := range source {
		id := strings.TrimSpace(item.ID)
		code := strings.TrimSpace(item.Number)
		reason := ""
		switch {
		case id == "":
			reason = "missing id"
		case code == "":
			reason = "missing number"
		}
		if reason != "" {
			slog.WarnContext(ctx, "skipping invalid JavDB recommendation",
				"movie_id", movieID, "field", field, "index", index,
				"reference_id", id, "reason", reason)
			continue
		}
		result = append(result, domain.MovieReference{ID: codeid.Normalize(code), Code: code, Thumbnail: item.ThumbURL, Cover: item.CoverURL})
	}
	return result
}

func moviesFromWire(ctx context.Context, source []wireMovie) ([]domain.Movie, error) {
	movies := make([]domain.Movie, len(source))
	for index, item := range source {
		movie, err := movieFromWire(ctx, item)
		if err != nil {
			return nil, fmt.Errorf("decode JavDB movie %d: %w", index, err)
		}
		movies[index] = movie
	}
	return movies, nil
}

func movieFromWire(ctx context.Context, source wireMovie) (domain.Movie, error) {
	if strings.TrimSpace(source.ID) == "" {
		return domain.Movie{}, errors.New("missing id")
	}
	code := strings.TrimSpace(source.Number)
	if code == "" {
		return domain.Movie{}, errors.New("missing number")
	}

	movie := domain.Movie{
		Sources:      []domain.SourceID{{Provider: "javdb", ID: source.ID}},
		RatingSource: "javdb", RatingMax: 5,
		ID:            source.ID,
		Code:          code,
		Title:         source.Title,
		OriginTitle:   source.OriginTitle,
		Plot:          strings.TrimSpace(source.Summary),
		ReleaseDate:   source.ReleaseDate,
		Duration:      source.Duration,
		Rating:        float64(source.Score),
		Thumbnail:     source.ThumbURL,
		Cover:         source.CoverURL,
		PreviewVideo:  source.PreviewVideoURL,
		MagnetsCount:  source.MagnetsCount,
		HasSubtitle:   source.HasCNSub,
		HasPreview:    source.HasPreviewImages || source.HasPreviewVideo,
		PreviewImages: make([]domain.PreviewImage, 0, len(source.PreviewImages)),
		Actors:        make([]domain.Actor, len(source.Actors)),
		Tags:          make([]domain.Tag, len(source.Tags)),
	}
	for _, image := range source.PreviewImages {
		if image.ThumbURL == "" && image.LargeURL == "" {
			continue
		}
		movie.PreviewImages = append(movie.PreviewImages, domain.PreviewImage{
			Thumbnail: image.ThumbURL,
			Original:  image.LargeURL,
		})
	}
	for index, actor := range source.Actors {
		gender := "unknown"
		if actor.Gender != nil {
			switch *actor.Gender {
			case 0:
				gender = "female"
			case 1:
				gender = "male"
			default:
				slog.WarnContext(ctx, "unknown JavDB actor gender; using unknown",
					"movie_id", movie.ID, "field", "actors.gender", "index", index,
					"actor_id", actor.ID, "value", *actor.Gender)
			}
		}
		movie.Actors[index] = domain.Actor{Provider: "javdb",
			ID:      actor.ID,
			Name:    actor.Name,
			NameZHT: actor.NameZHT,
			Gender:  gender,
			Avatar:  actor.AvatarURL,
		}
	}
	for index, tag := range source.Tags {
		movie.Tags[index] = domain.Tag{Provider: "javdb",
			ID:         tag.ID,
			Name:       tag.Name,
			NameZHT:    tag.NameZHT,
			CategoryID: tag.CategoryID,
		}
	}
	if source.SeriesID != "" || source.SeriesName != "" {
		movie.Series = &domain.Series{Provider: "javdb", ID: source.SeriesID, Name: source.SeriesName}
	}
	if source.MakerID != "" || source.MakerName != "" {
		movie.Maker = &domain.Maker{Provider: "javdb", ID: source.MakerID, Name: source.MakerName}
	}
	if source.DirectorID != "" || source.DirectorName != "" {
		movie.Director = &domain.Director{Provider: "javdb", ID: source.DirectorID, Name: source.DirectorName}
	}
	return movie, nil
}
