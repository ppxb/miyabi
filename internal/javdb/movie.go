package javdb

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
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
// Candidates from codeid.Candidates are matched in priority order, so an exact
// spelling wins over a relaxed one such as 326IHD-005 -> IHD-005. Each candidate
// accepts format-equivalent numbers (ABC00123 vs ABC-123 or ABP-001 vs ABP-1).
// Duplicate rows for the same ID are allowed; multiple distinct matching movies
// for one candidate are strictly rejected to prevent ambiguity.
func (c *Client) ResolveMovieID(ctx context.Context, number string) (string, error) {
	candidates := codeid.Candidates(number)
	if len(candidates) == 0 {
		return "", errors.New("catalogue number is required")
	}

	// Search results are fuzzy, so every response is checked against all
	// candidates before issuing the next query.
	queries := slices.Clone(candidates)
	for _, candidate := range candidates {
		if unpadded, ok := codeid.UnpaddedNumericCandidate(candidate); ok {
			queries = append(queries, unpadded)
		}
	}
	var movies []domain.Movie
	for _, query := range queries {
		results, err := c.Search(ctx, query, domain.SearchOptions{
			Zone:  domain.ZoneAll,
			Page:  1,
			Limit: 100,
		})
		if err != nil {
			return "", err
		}
		movies = append(movies, results...)
		for _, candidate := range candidates {
			if matched, err := matchCandidate(movies, candidate); err != nil || matched != "" {
				return matched, err
			}
		}
	}

	return "", fmt.Errorf("catalogue number %s was not found on JavDB", candidates[0])
}

func matchCandidate(movies []domain.Movie, wanted string) (string, error) {
	var exactMatched string
	exactIDs := make(map[string]struct{})
	for _, movie := range movies {
		if codeid.Normalize(movie.Code) == wanted {
			if _, ok := exactIDs[movie.ID]; !ok {
				exactIDs[movie.ID] = struct{}{}
				exactMatched = movie.ID
			}
		}
	}
	if len(exactIDs) > 1 {
		return "", fmt.Errorf("catalogue number %s has multiple exact JavDB matches", wanted)
	}
	if len(exactIDs) == 1 {
		return exactMatched, nil
	}

	var equivMatched string
	equivIDs := make(map[string]struct{})
	for _, movie := range movies {
		if codeid.IsFormatEquivalent(movie.Code, wanted) {
			if _, ok := equivIDs[movie.ID]; !ok {
				equivIDs[movie.ID] = struct{}{}
				equivMatched = movie.ID
			}
		}
	}
	if len(equivIDs) > 1 {
		return "", fmt.Errorf("catalogue number %s has multiple format-equivalent JavDB matches", wanted)
	}
	if len(equivIDs) == 1 {
		return equivMatched, nil
	}

	return "", nil
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
