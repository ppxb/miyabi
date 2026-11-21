package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/catalogue"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
)

type CatalogueManager interface {
	Browse(context.Context, domain.BrowseOptions) ([]catalogue.Movie, error)
	MovieDetail(context.Context, string) (catalogue.MovieDetail, error)
	MovieStates(context.Context, []catalogue.MovieIdentity) ([]catalogue.MovieStateItem, error)
	Magnets(context.Context, string) ([]catalogue.Magnet, error)
	MagnetsByCode(context.Context, string) ([]catalogue.Magnet, error)
	Tags(context.Context, domain.Zone) ([]domain.TagCategory, error)
	Media(context.Context, string) (domain.Media, error)
	LocalRecommendations(context.Context, domain.MovieMetadata) ([]domain.MovieReference, []domain.MovieReference, error)
	MagnetSourceSettings() []catalogue.MagnetSourceSetting
	UpdateMagnetSourceSettings(context.Context, []catalogue.MagnetSourceSetting) error
}

type ViewedManager interface {
	ViewedMovieIDs(context.Context) ([]string, error)
	AddViewedMovieIDs(context.Context, []string) error
}

type discoverBrowseQuery struct {
	Zone       string   `form:"zone" binding:"excluded_with=EntityType,omitempty,oneof=censored uncensored western fc2 anime"`
	EntityType string   `form:"entity_type" binding:"required_with=EntityID,omitempty,oneof=actor series maker director"`
	EntityID   string   `form:"entity_id" binding:"required_with=EntityType"`
	Main       []string `form:"main" binding:"omitempty,dive,oneof=p m c s i v"`
	TagIDs     []string `form:"tag_id" binding:"excluded_with=EntityType,omitempty,dive,required"`
	Year       string   `form:"year" binding:"excluded_with=EntityType,omitempty,len=4,numeric"`
	Month      string   `form:"month" binding:"excluded_with=EntityType,omitempty,oneof=1 2 3 4 5 6 7 8 9 10 11 12"`
	Sort       string   `form:"sort,default=hit" binding:"oneof=hit release score update want_watch_count watched_count"`
	Order      string   `form:"order,default=desc" binding:"oneof=asc desc"`
	Page       int      `form:"page,default=1" binding:"min=1"`
	Limit      int      `form:"limit,default=20" binding:"min=1,max=100"`
}

type discoverTagsQuery struct {
	Zone string `form:"zone,default=censored" binding:"oneof=censored uncensored western fc2 anime"`
}

type movieURI struct {
	ID string `uri:"id" binding:"required"`
}

type movieStatesInput struct {
	Movies []catalogue.MovieIdentity `json:"movies" binding:"required,min=1,max=100,dive"`
}

func discoverMovieStatesHandler(discover CatalogueManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		input, ok := bindJSON[movieStatesInput](c)
		if !ok {
			return
		}
		states, err := discover.MovieStates(c.Request.Context(), input.Movies)
		respond(c, states, err)
	}
}

type imageQuery struct {
	URL string `form:"url" binding:"required,url"`
}

func discoverBrowseHandler(discover CatalogueManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[discoverBrowseQuery](c)
		if !ok {
			return
		}
		movies, err := discover.Browse(c.Request.Context(), domain.BrowseOptions{
			Zone:       domain.Zone(query.Zone),
			EntityType: domain.EntityType(query.EntityType),
			EntityID:   query.EntityID,
			Main:       query.Main,
			TagIDs:     query.TagIDs,
			Year:       query.Year,
			Month:      query.Month,
			Sort:       query.Sort,
			Order:      query.Order,
			Page:       query.Page,
			Limit:      query.Limit,
		})
		respond(c, movies, err)
	}
}

type discoverMovieQuery struct {
	Code string `form:"code" binding:"omitempty,max=200"`
}

func discoverMovieHandler(discover CatalogueManager, metadata MetadataManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[movieURI](c)
		if !ok {
			return
		}
		query, ok := bindQuery[discoverMovieQuery](c)
		if !ok {
			return
		}
		// The discover identity is the normalized code. Detail resolves from the
		// multi-source metadata layer with JavDB excluded, using the explicit
		// code when supplied and falling back to the path id otherwise.
		code := codeid.Normalize(query.Code)
		if code == "" {
			code = codeid.Normalize(uri.ID)
		}
		if code != "" && codeid.Valid(code) {
			meta, err := metadata.ResolveExcept(c.Request.Context(), domain.MovieRef{Code: code}, "javdb")
			if err != nil {
				respond(c, nil, err)
				return
			}
			detail := catalogue.ProjectMetadataDetail(meta)
			// Recommendations come solely from already-indexed data, matched by
			// this movie's metadata; never from the source's suggestion lists.
			if actors, related, recErr := discover.LocalRecommendations(c.Request.Context(), meta); recErr != nil {
				c.Error(recErr)
			} else {
				detail.ActorMovies = actors
				detail.RelatedMovies = related
			}
			respond(c, detail, nil)
			return
		}
		movie, err := discover.MovieDetail(c.Request.Context(), uri.ID)
		respond(c, movie, err)
	}
}

func discoverTagsHandler(discover CatalogueManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[discoverTagsQuery](c)
		if !ok {
			return
		}
		categories, err := discover.Tags(c.Request.Context(), domain.Zone(query.Zone))
		respond(c, categories, err)
	}
}

type discoverMagnetsQuery struct {
	Code string `form:"code" binding:"omitempty,max=200"`
}

func discoverMagnetsHandler(discover CatalogueManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[movieURI](c)
		if !ok {
			return
		}
		query, ok := bindQuery[discoverMagnetsQuery](c)
		if !ok {
			return
		}
		// A known code lets magnets bypass the JavDB detail lookup; without it we
		// still fall back to the movie-ID path for older clients.
		var magnets []catalogue.Magnet
		var err error
		if code := codeid.Normalize(query.Code); code != "" && codeid.Valid(code) {
			magnets, err = discover.MagnetsByCode(c.Request.Context(), code)
		} else {
			magnets, err = discover.Magnets(c.Request.Context(), uri.ID)
		}
		respond(c, magnets, err)
	}
}

func imageHandler(discover CatalogueManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[imageQuery](c)
		if !ok {
			return
		}
		media, err := discover.Media(c.Request.Context(), query.URL)
		if err != nil {
			c.Error(err)
			return
		}
		c.Header("Cache-Control", "public, max-age=86400")
		c.Data(http.StatusOK, media.ContentType, media.Body)
	}
}

type addViewedInput struct {
	IDs []string `json:"ids" binding:"required,min=1,max=1000"`
}

func discoverViewedHandler(viewed ViewedManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		ids, err := viewed.ViewedMovieIDs(c.Request.Context())
		respond(c, ids, err)
	}
}

func discoverAddViewedHandler(viewed ViewedManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		input, ok := bindJSON[addViewedInput](c)
		if !ok {
			return
		}
		err := viewed.AddViewedMovieIDs(c.Request.Context(), input.IDs)
		respond(c, nil, err)
	}
}
