package api

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	sloggin "github.com/samber/slog-gin"
)

type HealthChecker interface {
	Ping(context.Context) error
}

type Dependencies struct {
	Logger   *slog.Logger
	Health   HealthChecker
	Discover Discoverer
	Frontend fs.FS
}

func NewRouter(deps Dependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(
		sloggin.New(deps.Logger),
		recoveryMiddleware(deps.Logger),
		errorMiddleware(deps.Logger),
	)

	api := router.Group("/api")
	api.GET("/health", healthHandler(deps.Health))
	api.GET("/discover/movies", discoverBrowseHandler(deps.Discover))
	api.GET("/discover/search", discoverSearchHandler(deps.Discover))
	api.GET("/discover/tags", discoverTagsHandler(deps.Discover))
	api.GET("/discover/movies/:id", discoverMovieHandler(deps.Discover))
	api.GET("/image", imageHandler(deps.Discover))
	api.GET("/javdb/route", javdbRouteHandler(deps.Discover))
	api.POST("/javdb/reselect", javdbReselectHandler(deps.Discover))

	if deps.Frontend != nil {
		installFrontend(router, deps.Frontend)
	}
	return router
}

func installFrontend(router *gin.Engine, frontend fs.FS) {
	fileServer := http.FileServer(http.FS(frontend))
	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		requestedPath := strings.TrimPrefix(c.Request.URL.Path, "/")
		if requestedPath != "" {
			if _, err := fs.Stat(frontend, requestedPath); err == nil {
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
		}

		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}
