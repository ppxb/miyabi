package api

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/source"
)

type SourceManager interface {
	List(context.Context) ([]source.Source, error)
	Replace(context.Context, []source.Source) ([]source.Source, error)
}

func sourceListHandler(sources SourceManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		list, err := sources.List(c.Request.Context())
		respond(c, list, err)
	}
}

func sourceUpdateHandler(sources SourceManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, ok := bindJSON[[]source.Source](c)
		if !ok {
			return
		}
		list, err := sources.Replace(c.Request.Context(), body)
		respond(c, list, err)
	}
}
