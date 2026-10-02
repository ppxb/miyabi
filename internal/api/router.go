package api

import (
	"bytes"
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
)

type HealthChecker interface {
	Ping(context.Context) error
}

type Dependencies struct {
	Logger         *slog.Logger
	Health         HealthChecker
	Access         AccessGate
	Catalogue      CatalogueManager
	Metadata       MetadataManager
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
	TrustedProxies []string
}

// NewRouter requires an Access gate, including when password protection is disabled.
func NewRouter(deps Dependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}

	if len(deps.TrustedProxies) > 0 {
		_ = router.SetTrustedProxies(deps.TrustedProxies)
	} else {
		_ = router.SetTrustedProxies(nil)
	}

	router.Use(
		requestLoggingMiddleware(deps.Logger),
		recoveryMiddleware(deps.Logger),
		errorMiddleware(deps.Logger),
		securityHeadersMiddleware(),
	)

	loginLimiter := newLoginRateLimiter(5, 5*time.Minute, 24*time.Hour)

	api := router.Group("/api", sameOriginMiddleware())
	api.GET("/health", healthHandler(deps.Health))

	authAPI := api.Group("/auth", noStore())
	authAPI.GET("/config", accessConfigHandler(deps.Access))
	authAPI.POST("/login", accessLoginHandler(deps.Access, loginLimiter))

	// Exported .strm files keep this path; it must stay stable across releases.
	strmHandler := strmStreamHandler(deps.STRM, deps.STRMToken, deps.Access)
	api.GET("/strm/play/:fileID", noStore(), strmHandler)
	api.HEAD("/strm/play/:fileID", noStore(), strmHandler)
	api.GET("/strm/play/:fileID/*filename", noStore(), strmHandler)
	api.HEAD("/strm/play/:fileID/*filename", noStore(), strmHandler)

	protected := api.Group("", authMiddleware(deps.Access))

	settingsAPI := protected.Group("/settings", noStore())
	settingsAPI.GET("/scraping", metadataSettingsHandler(deps.Metadata))
	settingsAPI.PUT("/scraping", metadataUpdateHandler(deps.Metadata))
	settingsAPI.GET("/system", dataInfoHandler(deps.Maintenance))
	settingsAPI.DELETE("/cache", dataClearCacheHandler(deps.Maintenance))
	settingsAPI.GET("/network", networkHandler(deps.Network))
	settingsAPI.PUT("/network", networkUpdateHandler(deps.Network))
	settingsAPI.POST("/network/test", networkTestHandler(deps.Network))
	settingsAPI.GET("/emby", embyConfigHandler(deps.Emby))
	settingsAPI.PUT("/emby", embyUpdateHandler(deps.Emby))
	settingsAPI.POST("/emby/test", embyTestHandler(deps.Emby))
	settingsAPI.GET("/subscription", subscriptionSettingsGetHandler(deps.Monitor))
	settingsAPI.PUT("/subscription", subscriptionSettingsUpdateHandler(deps.Monitor))

	protected.GET("/library/movies", libraryMoviesHandler(deps.Library))
	protected.GET("/library/movies/:id", libraryMovieHandler(deps.Library))
	protected.POST("/library/movies/:id/scrape", libraryMovieScrapeHandler(deps.Library))
	protected.GET("/library/movies/:id/previews/:index", libraryPreviewHandler(deps.Library, deps.Metadata))
	protected.POST("/library/scan", libraryScanHandler(deps.Library, deps.Emby))
	protected.POST("/library/rebuild", libraryRebuildHandler(deps.Library))
	protected.GET("/library/artwork/:key", libraryArtworkHandler(deps.Artwork))

	protected.GET("/tasks", noStore(), tasksHandler(deps.Tasks))
	protected.POST("/tasks/:id/retry", taskRetryHandler(deps.Tasks))
	protected.PUT("/tasks/library-control", taskLibraryControlHandler(deps.Tasks))
	protected.GET("/tasks/events", taskEventsHandler(deps.Tasks))
	protected.GET("/offline/tasks", noStore(), offlineActivityHandler(deps.Offline))

	subscriptionsAPI := protected.Group("/subscriptions", noStore())
	subscriptionsAPI.GET("", subscriptionListHandler(deps.Monitor))
	subscriptionsAPI.GET("/targets", subscriptionTargetsHandler(deps.Monitor))
	subscriptionsAPI.POST("", subscriptionCreateHandler(deps.Monitor))
	subscriptionsAPI.PATCH("/:id", subscriptionUpdateHandler(deps.Monitor))
	subscriptionsAPI.DELETE("/:id", subscriptionRemoveHandler(deps.Monitor))
	subscriptionsAPI.POST("/:id/enqueue", subscriptionEnqueueSingleHandler(deps.Monitor))
	subscriptionsAPI.POST("/enqueue", subscriptionEnqueueBatchHandler(deps.Monitor))
	subscriptionsAPI.GET("/actors/feed", subscriptionActorFeedHandler(deps.Monitor))
	subscriptionsAPI.GET("/actors/:id/feed", subscriptionActorFeedHandler(deps.Monitor))

	protected.GET("/discover/movies", discoverBrowseHandler(deps.Catalogue))
	protected.POST("/discover/movie-states", noStore(), discoverMovieStatesHandler(deps.Catalogue))
	protected.GET("/discover/viewed", noStore(), discoverViewedHandler(deps.Library))
	protected.POST("/discover/viewed", discoverAddViewedHandler(deps.Library))
	protected.GET("/discover/search", discoverSearchHandler(deps.Catalogue))
	protected.GET("/discover/tags", discoverTagsHandler(deps.Catalogue))
	protected.GET("/discover/movies/:id", discoverMovieHandler(deps.Catalogue))
	protected.GET("/discover/movies/resolve", discoverResolveMovieHandler(deps.Catalogue))
	protected.GET("/discover/movies/:id/magnets", discoverMagnetsHandler(deps.Catalogue))
	protected.POST("/discover/movies/:id/offline", offlineAddHandler(deps.Offline))
	protected.GET("/image", imageHandler(deps.Catalogue))

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

// Vite's default output names contain an eight-character content hash.
var hashedFrontendAsset = regexp.MustCompile(`^assets/.+-[A-Za-z0-9_-]{8}\.[^/]+$`)

func installFrontend(router *gin.Engine, frontend fs.FS) {
	fileServer := http.FileServer(http.FS(frontend))
	indexHTML, indexErr := fs.ReadFile(frontend, "index.html")
	serveIndex := func(c *gin.Context) {
		if indexErr != nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		http.ServeContent(c.Writer, c.Request, "index.html", time.Time{}, bytes.NewReader(indexHTML))
	}
	router.NoRoute(func(c *gin.Context) {
		if c.Request.URL.Path == "/api" || strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Error(domain.E(domain.KindNotFound, "not found", nil))
			return
		}
		c.Header("Cache-Control", "no-cache")
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Header("Allow", "GET, HEAD")
			c.AbortWithStatus(http.StatusMethodNotAllowed)
			return
		}

		requestedPath := strings.TrimSuffix(strings.TrimPrefix(c.Request.URL.Path, "/"), "/")
		if requestedPath == "" || requestedPath == "index.html" {
			serveIndex(c)
			return
		}
		if !fs.ValidPath(requestedPath) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		if info, err := fs.Stat(frontend, requestedPath); err == nil {
			if info.IsDir() {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}
			if hashedFrontendAsset.MatchString(requestedPath) {
				c.Header("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}
		// Missing assets must not receive an HTML response or immutable caching.
		if requestedPath == "assets" || strings.HasPrefix(requestedPath, "assets/") || path.Ext(requestedPath) != "" {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		serveIndex(c)
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
