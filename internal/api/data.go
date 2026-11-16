package api

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/maintenance"
)

type MaintenanceManager interface {
	Info(context.Context) (maintenance.Info, error)
	ClearCache(context.Context) (maintenance.Info, error)
}

type LogReader interface {
	Logs(context.Context, int) (maintenance.LogInfo, error)
}

func dataLogsHandler(logs LogReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := 500
		if value := c.Query("limit"); value != "" {
			if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		info, err := logs.Logs(c.Request.Context(), limit)
		respond(c, info, err)
	}
}

func dataInfoHandler(data MaintenanceManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		info, err := data.Info(c.Request.Context())
		respond(c, info, err)
	}
}

func dataClearCacheHandler(data MaintenanceManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		info, err := data.ClearCache(c.Request.Context())
		respond(c, info, err)
	}
}
