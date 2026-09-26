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
	if err := c.getJSON(
		ctx,
		"/api/v4/movies/"+url.PathEscape(movieID),
		nil,
		defaultLanguage,
		&data,
	); err != nil {
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
		result = append(result, domain.MovieReference{ID: id, Code: code, Thumbnail: item.ThumbURL})
	}
	return result
}

// ResolveMovieID finds the single distinct movie ID matching the catalogue number.
// Candidates from codeid.Layers are matched in priority order:
// Level 0: 完整原貌 (Complete normalized catalogue candidate)
// Level 1: 前缀回退 (Prefix stripped: distributor label digits or studio prefix removed)
// Level 2: 后缀回退 (Suffix stripped: video part or subtitle marker removed)
// Level 3: 双端回退 (Both prefix and suffix stripped)
//
// Higher-level candidates strictly win over lower-level ones.
// In each layer, exact normalized matches strictly win over format-equivalent ones.
// Duplicate rows for the same ID are allowed; multiple distinct matching movies
// for the same level are strictly rejected to prevent ambiguity.
func (c *Client) ResolveMovieID(ctx context.Context, number string) (string, error) {
	layers := codeid.Layers(number)
	if len(layers) == 0 {
		return "", errors.New("catalogue number is required")
	}

	var movies []domain.Movie
	queried := make(map[string]bool)

	for level, candidates := range layers {
		for _, candidate := range candidates {
			for _, query := range codeid.Queries(candidate) {
				// If candidate is already satisfied by existing search results,
				// do not issue redundant network requests.
				id, err := matchCandidates(movies, []string{candidate}, false)
				if err != nil {
					return "", err
				}
				if id != "" {
					break
				}
				if queried[query] {
					continue
				}
				queried[query] = true

				results, err := c.Search(ctx, query, domain.SearchOptions{
					Zone:  domain.ZoneAll,
					Page:  1,
					Limit: 100,
				})
				if err != nil {
					return "", err
				}
				movies = append(movies, results...)
			}
		}

		// Recheck completed levels in priority order.
		// A later query may uncover a stronger candidate from an earlier layer.
		for _, completed := range layers[:level+1] {
			for _, equivalent := range []bool{false, true} {
				id, err := matchCandidates(movies, completed, equivalent)
				if err != nil || id != "" {
					return id, err
				}
			}
		}
	}

	return "", fmt.Errorf("catalogue number %s was not found on JavDB", layers[0][0])
}

func matchCandidates(movies []domain.Movie, candidates []string, equivalent bool) (string, error) {
	var matchedID string
	ids := make(map[string]struct{})
	for _, movie := range movies {
		for _, candidate := range candidates {
			var matches bool
			if !equivalent {
				matches = codeid.Normalize(movie.Code) == candidate
			} else {
				matches = codeid.IsFormatEquivalent(movie.Code, candidate)
			}
			if matches {
				if _, ok := ids[movie.ID]; !ok {
					ids[movie.ID] = struct{}{}
					matchedID = movie.ID
				}
			}
		}
	}
	if len(ids) > 1 {
		kind := "exact"
		if equivalent {
			kind = "format-equivalent"
		}
		return "", fmt.Errorf("catalogue number %s has multiple %s JavDB matches", candidates[0], kind)
	}
	return matchedID, nil
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
		movie.Actors[index] = domain.Actor{
			ID:      actor.ID,
			Name:    actor.Name,
			NameZHT: actor.NameZHT,
			Gender:  gender,
			Avatar:  actor.AvatarURL,
		}
	}
	for index, tag := range source.Tags {
		movie.Tags[index] = domain.Tag{
			ID:         tag.ID,
			Name:       tag.Name,
			NameZHT:    tag.NameZHT,
			CategoryID: tag.CategoryID,
		}
	}
	if source.SeriesID != "" || source.SeriesName != "" {
		movie.Series = &domain.Series{ID: source.SeriesID, Name: source.SeriesName}
	}
	if source.MakerID != "" || source.MakerName != "" {
		movie.Maker = &domain.Maker{ID: source.MakerID, Name: source.MakerName}
	}
	if source.DirectorID != "" || source.DirectorName != "" {
		movie.Director = &domain.Director{ID: source.DirectorID, Name: source.DirectorName}
	}
	return movie, nil
}
