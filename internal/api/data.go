package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/service"
)

type DataManager interface {
	Info(context.Context) (service.DataInfo, error)
	ClearCache(context.Context) (service.DataInfo, error)
}

func dataInfoHandler(data DataManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		info, err := data.Info(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, info)
	}
}

func dataClearCacheHandler(data DataManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		info, err := data.ClearCache(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, info)
	}
}
