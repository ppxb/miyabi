package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/service"
)

type Settings interface {
	Preferences(context.Context) (service.Preferences, error)
	SavePreferences(context.Context, service.Preferences) (service.Preferences, error)
}

type preferencesInput struct {
	NSFWMode *bool `json:"nsfw_mode" binding:"required"`
}

func preferencesHandler(settings Settings) gin.HandlerFunc {
	return func(c *gin.Context) {
		preferences, err := settings.Preferences(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, preferences)
	}
}

func savePreferencesHandler(settings Settings) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input preferencesInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		preferences, err := settings.SavePreferences(c.Request.Context(), service.Preferences{NSFWMode: *input.NSFWMode})
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, preferences)
	}
}
