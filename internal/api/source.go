package api

import (
	"context"
	"log/slog"
	"maps"

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
// every enabled source when the save can change what a scan covers. A failed
// enqueue is logged and does not fail the save.
func sourceUpdateHandler(sources SourceManager, library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, ok := bindJSON[[]source.Source](c)
		if !ok {
			return
		}
		ctx := c.Request.Context()
		before, listErr := sources.List(ctx)
		list, err := sources.Replace(ctx, body)
		if err == nil && (listErr != nil || scanTargetsChanged(before, list)) {
			if _, scanErr := library.StartIncrementalSync(ctx); scanErr != nil {
				slog.WarnContext(ctx, "queue scans after saving sources failed", "error", scanErr)
			}
		}
		respond(c, list, err)
	}
}

// scanTargetsChanged reports whether a save adds or removes a source or edits
// its kind, enabled flag or config. Renaming or reordering alone does not.
func scanTargetsChanged(before, after []source.Source) bool {
	if len(before) != len(after) {
		return true
	}
	previous := make(map[string]source.Source, len(before))
	for _, src := range before {
		previous[src.ID] = src
	}
	for _, src := range after {
		old, ok := previous[src.ID]
		if !ok || old.Kind != src.Kind || old.Enabled != src.Enabled || !maps.Equal(old.Config, src.Config) {
			return true
		}
	}
	return false
}
