package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/catalogue"
	"github.com/ppxb/miyabi/internal/domain"
)

type CatalogueManager interface {
	Search(context.Context, string, domain.SearchOptions) ([]catalogue.Movie, error)
	Browse(context.Context, domain.BrowseOptions) ([]catalogue.Movie, error)
	MovieDetail(context.Context, string) (catalogue.MovieDetail, error)
	MovieStates(context.Context, []catalogue.MovieIdentity) ([]catalogue.MovieStateItem, error)
	Magnets(context.Context, string) ([]catalogue.Magnet, error)
	Tags(context.Context, domain.Zone) ([]domain.TagCategory, error)
	Media(context.Context, string) (domain.Media, error)
	Route() catalogue.RouteStatus
	Reselect(context.Context) (catalogue.RouteStatus, error)
	SelectRoute(context.Context, string) (catalogue.RouteStatus, error)
}

type ViewedManager interface {
	ViewedMovieIDs(context.Context, ...int) ([]string, error)
	AddViewedMovieIDs(context.Context, []string) error
}

type discoverSearchQuery struct {
	Query string `form:"q" binding:"required"`
	Page  int    `form:"page,default=1" binding:"min=1"`
	Limit int    `form:"limit,default=20" binding:"min=1,max=100"`
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

type javdbRouteInput struct {
	Host string `json:"host" binding:"omitempty,url"`
}

func discoverSearchHandler(discover CatalogueManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[discoverSearchQuery](c)
		if !ok {
			return
		}
		movies, err := discover.Search(c.Request.Context(), query.Query, domain.SearchOptions{
			Page:  query.Page,
			Limit: query.Limit,
		})
		respond(c, movies, err)
	}
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

func discoverMovieHandler(discover CatalogueManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[movieURI](c)
		if !ok {
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

func discoverMagnetsHandler(discover CatalogueManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[movieURI](c)
		if !ok {
			return
		}
		magnets, err := discover.Magnets(c.Request.Context(), uri.ID)
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

func javdbRouteHandler(discover CatalogueManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		respond(c, discover.Route(), nil)
	}
}

func javdbReselectHandler(discover CatalogueManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		status, err := discover.Reselect(c.Request.Context())
		respond(c, status, err)
	}
}

func javdbSelectRouteHandler(discover CatalogueManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		input, ok := bindJSON[javdbRouteInput](c)
		if !ok {
			return
		}
		status, err := discover.SelectRoute(c.Request.Context(), input.Host)
		respond(c, status, err)
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
