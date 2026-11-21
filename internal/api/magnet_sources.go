package api

import (
	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/catalogue"
)

func magnetSourceSettingsHandler(catalogueMgr CatalogueManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		respond(c, catalogueMgr.MagnetSourceSettings(), nil)
	}
}

func magnetSourceUpdateHandler(catalogueMgr CatalogueManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, ok := bindJSON[[]catalogue.MagnetSourceSetting](c)
		if !ok {
			return
		}
		if err := catalogueMgr.UpdateMagnetSourceSettings(c.Request.Context(), settings); err != nil {
			c.Error(err)
			return
		}
		respond(c, catalogueMgr.MagnetSourceSettings(), nil)
	}
}
