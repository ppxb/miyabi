package api

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/emby"
)

// EmbyManager defines the contract for reading, saving, and testing Emby settings.
type EmbyManager interface {
	Config(context.Context) (emby.Config, error)
	RetryPending(context.Context) error
	UpdateConfig(context.Context, emby.Config) error
	Test(context.Context, emby.Config) (emby.ServerInfo, error)
}

func embyConfigHandler(manager EmbyManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, err := manager.Config(c.Request.Context())
		respond(c, cfg, err)
	}
}

func embyUpdateHandler(manager EmbyManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, ok := bindJSON[emby.Config](c)
		if !ok {
			return
		}
		if err := manager.UpdateConfig(c.Request.Context(), cfg); err != nil {
			c.Error(err)
			return
		}
		updated, err := manager.Config(c.Request.Context())
		respond(c, updated, err)
	}
}

func embyTestHandler(manager EmbyManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var cfg emby.Config
		if c.Request.ContentLength != 0 {
			bodyConfig, ok := bindJSON[emby.Config](c)
			if !ok {
				return
			}
			cfg = bodyConfig
		} else {
			var err error
			cfg, err = manager.Config(c.Request.Context())
			if err != nil {
				c.Error(err)
				return
			}
		}
		info, err := manager.Test(c.Request.Context(), cfg)
		respond(c, info, err)
	}
}
