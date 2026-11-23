package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ppxb/miyabi/internal/actors"
	"github.com/ppxb/miyabi/internal/domain"
	lib "github.com/ppxb/miyabi/internal/library"
)

// ActorManager exposes the canonical actor read model.
type ActorManager interface {
	Catalog(context.Context, string, int, int) (actors.Page, error)
	Detail(context.Context, int) (actors.Detail, error)
	FetchImage(context.Context, int, int) ([]byte, error)
	FetchAvatar(context.Context, int) ([]byte, error)
	Settings(context.Context) ([]domain.SourceToggle, error)
	UpdateSettings(context.Context, []domain.SourceToggle) error
}

func actorSourceSettingsHandler(manager ActorManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, err := manager.Settings(c.Request.Context())
		respond(c, settings, err)
	}
}

func actorSourceUpdateHandler(manager ActorManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, ok := bindJSON[[]domain.SourceToggle](c)
		if !ok {
			return
		}
		if err := manager.UpdateSettings(c.Request.Context(), settings); err != nil {
			c.Error(err)
			return
		}
		result, err := manager.Settings(c.Request.Context())
		respond(c, result, err)
	}
}

func actorListHandler(manager ActorManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[struct {
			Page  int `form:"page,default=1" binding:"min=1"`
			Limit int `form:"limit,default=24" binding:"min=1,max=100"`
		}](c)
		if !ok {
			return
		}
		result, err := manager.Catalog(c.Request.Context(), c.Query("q"), query.Page, query.Limit)
		respond(c, result, err)
	}
}

// actorURI is the canonical actor id (ActorProfile id) in /api/actors/:key.
type actorURI struct {
	ID int `uri:"key" binding:"required,min=1"`
}

type actorDetailResponse struct {
	actors.Detail
	Films lib.Page `json:"films"`
}

func actorDetailHandler(manager ActorManager, library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[actorURI](c)
		if !ok {
			return
		}
		detail, err := manager.Detail(c.Request.Context(), uri.ID)
		if err != nil {
			if errors.Is(err, actors.ErrNotFound) {
				c.Error(domain.E(domain.KindNotFound, "演员不存在", nil))
				return
			}
			c.Error(err)
			return
		}
		films, err := library.BrowseMovies(c.Request.Context(), lib.BrowseFilter{
			ActorNames: detail.Names,
			Sort:       "release",
			Page:       1,
			Limit:      100,
		})
		if err != nil {
			c.Error(err)
			return
		}
		respond(c, actorDetailResponse{Detail: detail, Films: films}, nil)
	}
}

func actorImageHandler(manager ActorManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[struct {
			actorURI
			Index int `uri:"index" binding:"min=0,max=999"`
		}](c)
		if !ok {
			return
		}
		data, err := manager.FetchImage(c.Request.Context(), uri.ID, uri.Index)
		writeActorImage(c, data, err)
	}
}

func actorAvatarHandler(manager ActorManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[actorURI](c)
		if !ok {
			return
		}
		data, err := manager.FetchAvatar(c.Request.Context(), uri.ID)
		writeActorImage(c, data, err)
	}
}

func writeActorImage(c *gin.Context, data []byte, err error) {
	if err != nil {
		if errors.Is(err, actors.ErrNotFound) {
			c.Status(http.StatusNotFound)
			return
		}
		c.Error(err)
		return
	}
	c.Header("Cache-Control", "private, max-age=86400")
	c.Data(http.StatusOK, http.DetectContentType(data), data)
}
