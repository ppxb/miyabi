package javdb

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
)

// MovieDetail fetches a movie and its graph metadata.
func (c *Client) MovieDetail(ctx context.Context, movieID string) (MovieDetail, error) {
	movieID = strings.TrimSpace(movieID)
	if movieID == "" {
		return MovieDetail{}, errors.New("JavDB movie ID is required")
	}

	var data wireMovieData
	if err := c.getJSON(
		ctx,
		"/api/v4/movies/"+url.PathEscape(movieID),
		nil,
		defaultLanguage,
		&data,
	); err != nil {
		return MovieDetail{}, err
	}
	movie, err := movieFromWire(data.Movie.wireMovie)
	if err != nil {
		return MovieDetail{}, err
	}
	zone := ZoneUnknown
	if data.Movie.Type != nil {
		zone = zoneFromCode(*data.Movie.Type)
		if zone == ZoneUnknown {
			slog.WarnContext(ctx, "unknown JavDB movie type; using unknown zone",
				"movie_id", movie.ID, "field", "type", "value", *data.Movie.Type)
		}
	}
	return MovieDetail{
		Movie: movie, Zone: zone,
		ActorMovies:   movieReferencesFromWire(movie.ID, "actor_movies", data.Movie.ActorMovies),
		RelatedMovies: movieReferencesFromWire(movie.ID, "relative_movies", data.Movie.RelatedMovies),
	}, nil
}

func zoneFromCode(code int) Zone {
	for zone, value := range zoneCodes {
		if value == code {
			return zone
		}
	}
	return ZoneUnknown
}

func movieReferencesFromWire(movieID, field string, source []wireMovieReference) []MovieReference {
	result := make([]MovieReference, 0, len(source))
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
			slog.Warn("skipping invalid JavDB recommendation",
				"movie_id", movieID, "field", field, "index", index,
				"reference_id", id, "reason", reason)
			continue
		}
		result = append(result, MovieReference{ID: id, Code: code, Thumbnail: item.ThumbURL})
	}
	return result
}

// ResolveMovieID finds the single exact catalogue-number match returned by
// JavDB search. Similar search hits are never accepted.
func (c *Client) ResolveMovieID(ctx context.Context, number string) (string, error) {
	wanted := codeid.Normalize(number)
	if wanted == "" {
		return "", errors.New("catalogue number is required")
	}

	movies, err := c.Search(ctx, wanted, SearchOptions{
		Zone:  ZoneAll,
		Page:  1,
		Limit: 100,
	})
	if err != nil {
		return "", err
	}

	var matched string
	for _, movie := range movies {
		if codeid.Normalize(movie.Code) != wanted {
			continue
		}
		if matched != "" {
			return "", fmt.Errorf("catalogue number %s has multiple exact JavDB matches", wanted)
		}
		matched = movie.ID
	}
	if matched == "" {
		return "", fmt.Errorf("catalogue number %s was not found on JavDB", wanted)
	}
	return matched, nil
}

func moviesFromWire(source []wireMovie) ([]Movie, error) {
	movies := make([]Movie, len(source))
	for index, item := range source {
		movie, err := movieFromWire(item)
		if err != nil {
			return nil, fmt.Errorf("decode JavDB movie %d: %w", index, err)
		}
		movies[index] = movie
	}
	return movies, nil
}

func movieFromWire(source wireMovie) (Movie, error) {
	if strings.TrimSpace(source.ID) == "" {
		return Movie{}, errors.New("missing id")
	}
	code := strings.TrimSpace(source.Number)
	if code == "" {
		return Movie{}, errors.New("missing number")
	}

	movie := Movie{
		ID:            source.ID,
		Code:          code,
		Title:         source.Title,
		OriginTitle:   source.OriginTitle,
		ReleaseDate:   source.ReleaseDate,
		Duration:      source.Duration,
		Rating:        float64(source.Score),
		Thumbnail:     source.ThumbURL,
		Cover:         source.CoverURL,
		PreviewVideo:  source.PreviewVideoURL,
		MagnetsCount:  source.MagnetsCount,
		HasSubtitle:   source.HasCNSub,
		HasPreview:    source.HasPreviewImages || source.HasPreviewVideo,
		PreviewImages: make([]PreviewImage, 0, len(source.PreviewImages)),
		Actors:        make([]Actor, len(source.Actors)),
		Tags:          make([]Tag, len(source.Tags)),
	}
	for _, image := range source.PreviewImages {
		if image.ThumbURL == "" && image.LargeURL == "" {
			continue
		}
		movie.PreviewImages = append(movie.PreviewImages, PreviewImage{
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
				slog.Warn("unknown JavDB actor gender; using unknown",
					"movie_id", movie.ID, "field", "actors.gender", "index", index,
					"actor_id", actor.ID, "value", *actor.Gender)
			}
		}
		movie.Actors[index] = Actor{
			ID:      actor.ID,
			Name:    actor.Name,
			NameZHT: actor.NameZHT,
			Gender:  gender,
			Avatar:  actor.AvatarURL,
		}
	}
	for index, tag := range source.Tags {
		movie.Tags[index] = Tag{
			ID:         tag.ID,
			Name:       tag.Name,
			NameZHT:    tag.NameZHT,
			CategoryID: tag.CategoryID,
		}
	}
	if source.SeriesID != "" || source.SeriesName != "" {
		movie.Series = &Series{ID: source.SeriesID, Name: source.SeriesName}
	}
	if source.MakerID != "" || source.MakerName != "" {
		movie.Maker = &Maker{ID: source.MakerID, Name: source.MakerName}
	}
	if source.DirectorID != "" || source.DirectorName != "" {
		movie.Director = &Director{ID: source.DirectorID, Name: source.DirectorName}
	}
	return movie, nil
}
