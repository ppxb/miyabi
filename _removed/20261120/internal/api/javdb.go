package api

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/javdb"
)

// JavDBManager defines the contract for reading, saving, and testing the
// optional JavDB account used for authenticated search.
type JavDBManager interface {
	JavDBConfig(context.Context) (javdb.Config, error)
	UpdateJavDBConfig(context.Context, javdb.Config) error
	TestJavDBLogin(context.Context, javdb.Config) (javdb.LoginInfo, error)
}

func javdbConfigHandler(manager JavDBManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, err := manager.JavDBConfig(c.Request.Context())
		respond(c, cfg, err)
	}
}

func javdbUpdateHandler(manager JavDBManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, ok := bindJSON[javdb.Config](c)
		if !ok {
			return
		}
		if err := manager.UpdateJavDBConfig(c.Request.Context(), cfg); err != nil {
			c.Error(err)
			return
		}
		updated, err := manager.JavDBConfig(c.Request.Context())
		respond(c, updated, err)
	}
}

func javdbTestHandler(manager JavDBManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var cfg javdb.Config
		if c.Request.ContentLength != 0 {
			bodyConfig, ok := bindJSON[javdb.Config](c)
			if !ok {
				return
			}
			cfg = bodyConfig
		} else {
			var err error
			cfg, err = manager.JavDBConfig(c.Request.Context())
			if err != nil {
				c.Error(err)
				return
			}
		}
		info, err := manager.TestJavDBLogin(c.Request.Context(), cfg)
		respond(c, info, err)
	}
}
