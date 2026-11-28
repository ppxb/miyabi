package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/maintenance"
)

type MaintenanceManager interface {
	Info(context.Context) (maintenance.Info, error)
	RefreshInfo(context.Context) (maintenance.Info, error)
	ClearCache(context.Context) (maintenance.Info, error)
}

type LogReader interface {
	LogMeta(context.Context) (maintenance.LogMeta, error)
	Logs(context.Context, int) (maintenance.LogInfo, error)
}

func dataLogsHandler(logs LogReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		meta, err := logs.LogMeta(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		limit := 500
		if value := c.Query("limit"); value != "" {
			if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		etag := fmt.Sprintf(`"%x-%x-%d"`, meta.SizeBytes, meta.ModTime.UnixNano(), limit)
		// The settings group defaults to no-store; the log tail is safe to
		// revalidate so unchanged files skip both the read and the payload.
		c.Header("Cache-Control", "private, no-cache")
		c.Header("ETag", etag)
		if !meta.ModTime.IsZero() {
			c.Header("Last-Modified", meta.ModTime.UTC().Format(http.TimeFormat))
		}
		if c.GetHeader("If-None-Match") == etag {
			c.Status(http.StatusNotModified)
			return
		}
		info, err := logs.Logs(c.Request.Context(), limit)
		respond(c, info, err)
	}
}

func dataInfoHandler(data MaintenanceManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		read := data.Info
		if c.Query("refresh") == "1" {
			read = data.RefreshInfo
		}
		info, err := read(c.Request.Context())
		respond(c, info, err)
	}
}

func dataClearCacheHandler(data MaintenanceManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		info, err := data.ClearCache(c.Request.Context())
		respond(c, info, err)
	}
}
