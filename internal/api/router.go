package api

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
	sloggin "github.com/samber/slog-gin"
)

type HealthChecker interface {
	Ping(context.Context) error
}

type Dependencies struct {
	Logger         *slog.Logger
	Health         HealthChecker
	Access         AccessGate
	Catalogue      CatalogueManager
	Drive          DriveManager
	Offline        OfflineManager
	Monitor        SubscriptionManager
	Library        LibraryManager
	STRM           STRMRelay
	Tasks          TaskManager
	Artwork        ArtworkReader
	Maintenance    MaintenanceManager
	Network        NetworkManager
	Emby           EmbyManager
	Frontend       fs.FS
	STRMToken      string
	EmbyDir        string
	TrustedProxies []string
}

func NewRouter(deps Dependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	if len(deps.TrustedProxies) > 0 {
		_ = router.SetTrustedProxies(deps.TrustedProxies)
	} else {
		_ = router.SetTrustedProxies(nil)
	}

	router.Use(
		sloggin.NewWithFilters(deps.Logger, sloggin.IgnoreStatus(statusClientClosedRequest)),
		recoveryMiddleware(deps.Logger),
		errorMiddleware(deps.Logger),
		securityHeadersMiddleware(),
	)

	loginLimiter := newLoginRateLimiter(5, 5*time.Minute, 24*time.Hour)

	api := router.Group("/api")
	api.GET("/health", healthHandler(deps.Health))

	authAPI := api.Group("/auth", noStore())
	authAPI.GET("/config", accessConfigHandler(deps.Access))
	authAPI.POST("/login", accessLoginHandler(deps.Access, loginLimiter))
	authAPI.POST("/logout", accessLogoutHandler())

	// Exported .strm files keep this path; it must stay stable across releases.
	strmHandler := strmStreamHandler(deps.STRM, deps.STRMToken, deps.Access)
	api.GET("/strm/play/:fileID", noStore(), strmHandler)
	api.HEAD("/strm/play/:fileID", noStore(), strmHandler)

	protected := api.Group("", authMiddleware(deps.Access))

	settingsAPI := protected.Group("/settings", noStore())
	settingsAPI.GET("/system", dataInfoHandler(deps.Maintenance))
	settingsAPI.DELETE("/cache", dataClearCacheHandler(deps.Maintenance))
	settingsAPI.GET("/network", networkHandler(deps.Network))
	settingsAPI.PUT("/network", networkUpdateHandler(deps.Network))
	settingsAPI.POST("/network/test", networkTestHandler(deps.Network))
	settingsAPI.GET("/emby", embyConfigHandler(deps.Emby))
	settingsAPI.PUT("/emby", embyUpdateHandler(deps.Emby))
	settingsAPI.POST("/emby/test", embyTestHandler(deps.Emby))
	settingsAPI.GET("/javbus", javbusConfigHandler(deps.Catalogue))
	settingsAPI.PUT("/javbus", javbusUpdateHandler(deps.Catalogue))
	settingsAPI.GET("/subscription", subscriptionSettingsGetHandler(deps.Monitor))
	settingsAPI.PUT("/subscription", subscriptionSettingsUpdateHandler(deps.Monitor))

	protected.GET("/library/movies", libraryMoviesHandler(deps.Library))
	protected.POST("/library/scan", libraryScanHandler(deps.Library))
	protected.POST("/library/scan/local", libraryLocalScanHandler(deps.Library, deps.EmbyDir))
	protected.GET("/library/artwork/:key", libraryArtworkHandler(deps.Artwork))

	protected.GET("/tasks", noStore(), tasksHandler(deps.Tasks))
	protected.GET("/tasks/events", taskEventsHandler(deps.Tasks))
	protected.GET("/offline/tasks", noStore(), offlineActivityHandler(deps.Offline))

	subscriptionsAPI := protected.Group("/subscriptions", noStore())
	subscriptionsAPI.GET("", subscriptionListHandler(deps.Monitor))
	subscriptionsAPI.POST("", subscriptionCreateHandler(deps.Monitor))
	subscriptionsAPI.PATCH("/:id", subscriptionUpdateHandler(deps.Monitor))
	subscriptionsAPI.DELETE("/:id", subscriptionRemoveHandler(deps.Monitor))
	subscriptionsAPI.POST("/:id/enqueue", subscriptionEnqueueSingleHandler(deps.Monitor))
	subscriptionsAPI.POST("/enqueue", subscriptionEnqueueBatchHandler(deps.Monitor))
	subscriptionsAPI.GET("/actors/:id/feed", subscriptionActorFeedHandler(deps.Monitor))

	protected.GET("/discover/movies", discoverBrowseHandler(deps.Catalogue))
	protected.POST("/discover/movie-states", noStore(), discoverMovieStatesHandler(deps.Catalogue))
	protected.GET("/discover/viewed", noStore(), discoverViewedHandler(deps.Library))
	protected.POST("/discover/viewed", discoverAddViewedHandler(deps.Library))
	protected.GET("/discover/search", discoverSearchHandler(deps.Catalogue))
	protected.GET("/discover/tags", discoverTagsHandler(deps.Catalogue))
	protected.GET("/discover/movies/:id", discoverMovieHandler(deps.Catalogue))
	protected.GET("/discover/movies/:id/magnets", discoverMagnetsHandler(deps.Catalogue))
	protected.POST("/discover/movies/:id/offline", offlineAddHandler(deps.Offline))
	protected.GET("/discover/movies/:id/offline", noStore(), offlineTasksHandler(deps.Offline))
	protected.GET("/image", imageHandler(deps.Catalogue))
	protected.GET("/javdb/route", javdbRouteHandler(deps.Catalogue))
	protected.PUT("/javdb/route", javdbSelectRouteHandler(deps.Catalogue))
	protected.POST("/javdb/reselect", javdbReselectHandler(deps.Catalogue))

	panAPI := protected.Group("/pan", noStore())
	panAPI.GET("/account", panAccountHandler(deps.Drive))
	panAPI.DELETE("/account", panDisconnectHandler(deps.Drive))
	panAPI.POST("/login", panBeginLoginHandler(deps.Drive))
	panAPI.GET("/login/:id", panLoginStatusHandler(deps.Drive))
	panAPI.GET("/files", panFilesHandler(deps.Drive))
	panAPI.PUT("/directory", panSelectDirectoryHandler(deps.Drive))
	panAPI.DELETE("/directory", panClearDirectoryHandler(deps.Drive))

	if deps.Frontend != nil {
		installFrontend(router, deps.Frontend)
	}
	return router
}

func installFrontend(router *gin.Engine, frontend fs.FS) {
	fileServer := http.FileServer(http.FS(frontend))
	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Error(domain.E(domain.KindNotFound, "not found", nil))
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

func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}
