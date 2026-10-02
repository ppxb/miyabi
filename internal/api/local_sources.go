package api

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/localsource"
)

// LocalSourceManager manages the local media directories scanned alongside the
// mounted 115 library.
type LocalSourceManager interface {
	List(context.Context) ([]localsource.Source, error)
	Add(ctx context.Context, name, path string) (localsource.Source, error)
	Update(ctx context.Context, id string, update localsource.Update) (localsource.Source, error)
	Remove(ctx context.Context, id string) error
}

type localSourceURI struct {
	ID string `uri:"id" binding:"required,max=64"`
}

type localSourceInput struct {
	Name string `json:"name" binding:"max=120"`
	Path string `json:"path" binding:"required,max=4096"`
}

func localSourceListHandler(manager LocalSourceManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		sources, err := manager.List(c.Request.Context())
		respond(c, sources, err)
	}
}

func localSourceCreateHandler(manager LocalSourceManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		input, ok := bindJSON[localSourceInput](c)
		if !ok {
			return
		}
		source, err := manager.Add(c.Request.Context(), input.Name, input.Path)
		created(c, source, err)
	}
}

func localSourceUpdateHandler(manager LocalSourceManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[localSourceURI](c)
		if !ok {
			return
		}
		update, ok := bindJSON[localsource.Update](c)
		if !ok {
			return
		}
		source, err := manager.Update(c.Request.Context(), uri.ID, update)
		respond(c, source, err)
	}
}

func localSourceRemoveHandler(manager LocalSourceManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[localSourceURI](c)
		if !ok {
			return
		}
		respond(c, nil, manager.Remove(c.Request.Context(), uri.ID))
	}
}
