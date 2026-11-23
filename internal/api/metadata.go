package api

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
)

type MetadataManager interface {
	Settings() []domain.SourceToggle
	UpdateSettings(context.Context, []domain.SourceToggle) error
	ResolveExcept(context.Context, domain.MovieRef, ...string) (domain.MovieMetadata, error)
	Image(context.Context, domain.ImageCandidate) (domain.Media, error)
}

func metadataSettingsHandler(service MetadataManager) gin.HandlerFunc {
	return func(c *gin.Context) { respond(c, service.Settings(), nil) }
}
func metadataUpdateHandler(service MetadataManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, ok := bindJSON[[]domain.SourceToggle](c)
		if !ok {
			return
		}
		if err := service.UpdateSettings(c.Request.Context(), settings); err != nil {
			c.Error(err)
			return
		}
		respond(c, service.Settings(), nil)
	}
}
