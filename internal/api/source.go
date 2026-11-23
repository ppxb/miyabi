package api

import (
	"context"
	"log/slog"

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

// sourceUpdateHandler saves the sources, then queues an incremental scan for
// every enabled source. A failed enqueue is logged and does not fail the save.
func sourceUpdateHandler(sources SourceManager, library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, ok := bindJSON[[]source.Source](c)
		if !ok {
			return
		}
		ctx := c.Request.Context()
		list, err := sources.Replace(ctx, body)
		if err == nil {
			if _, scanErr := library.StartIncrementalSync(ctx); scanErr != nil {
				slog.WarnContext(ctx, "queue scans after saving sources failed", "error", scanErr)
			}
		}
		respond(c, list, err)
	}
}
