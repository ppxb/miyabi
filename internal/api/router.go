package api

import (
	"bytes"
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

type Dependencies struct {
	Logger         *slog.Logger
	Access         AccessGate
	Catalogue      CatalogueManager
	Metadata       MetadataManager
	Drive          DriveManager
	Offline        OfflineManager
	Monitor        SubscriptionManager
	Library        LibraryManager
	Play           PlayManager
	STRM           STRMRelay
	Tasks          TaskManager
	Artwork        ArtworkReader
	Maintenance    MaintenanceManager
	Network        NetworkManager
	Emby           EmbyManager
	Sources        SourceManager
	Actors         ActorManager
	Subtitles      SubtitleManager
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

	if err := router.SetTrustedProxies(deps.TrustedProxies); err != nil {
		// Config validation should prevent this; fall back to trusting no proxy.
		deps.Logger.Error("apply trusted proxies", "error", err)
		_ = router.SetTrustedProxies(nil)
	}

	router.Use(
		requestLoggingMiddleware(deps.Logger),
		requestDiagnosticsMiddleware(),
		recoveryMiddleware(deps.Logger),
		errorMiddleware(deps.Logger),
		securityHeadersMiddleware(),
	)

	loginLimiter := newLoginRateLimiter(5, 5*time.Minute, 24*time.Hour)

	api := router.Group("/api", sameOriginMiddleware())

	authAPI := api.Group("/auth", noStore())
	authAPI.GET("/config", accessConfigHandler(deps.Access))
	authAPI.POST("/login", accessLoginHandler(deps.Access, loginLimiter))

	// Exported .strm files keep this path; it must stay stable across releases.
	strmHandler := strmStreamHandler(deps.STRM, deps.STRMToken, deps.Access)
	api.GET("/strm/play/:fileID", noStore(), strmHandler)
	api.HEAD("/strm/play/:fileID", noStore(), strmHandler)
	api.GET("/strm/play/:fileID/*filename", noStore(), strmHandler)
	api.HEAD("/strm/play/:fileID/*filename", noStore(), strmHandler)

	// External players cannot send the session cookie; serve a subtitle as
	// UTF-8 text using the same token policy as the STRM relay.
	api.GET("/subtitle-file/:trackID", noStore(), subtitleFileHandler(deps.Subtitles, deps.STRMToken, deps.Access))

	protected := api.Group("", authMiddleware(deps.Access))

	settingsAPI := protected.Group("/settings", noStore())
	settingsAPI.GET("/scraping", metadataSettingsHandler(deps.Metadata))
	settingsAPI.PUT("/scraping", metadataUpdateHandler(deps.Metadata))
	settingsAPI.GET("/magnet-sources", magnetSourceSettingsHandler(deps.Catalogue))
	settingsAPI.PUT("/magnet-sources", magnetSourceUpdateHandler(deps.Catalogue))
	settingsAPI.GET("/actor-sources", actorSourceSettingsHandler(deps.Actors))
	settingsAPI.PUT("/actor-sources", actorSourceUpdateHandler(deps.Actors))
	settingsAPI.GET("/system", dataInfoHandler(deps.Maintenance))
	settingsAPI.DELETE("/cache", dataClearCacheHandler(deps.Maintenance))
	settingsAPI.GET("/network", networkHandler(deps.Network))
	settingsAPI.PUT("/network", networkUpdateHandler(deps.Network))
	settingsAPI.GET("/emby", embyConfigHandler(deps.Emby))
	settingsAPI.PUT("/emby", embyUpdateHandler(deps.Emby))
	settingsAPI.POST("/emby/test", embyTestHandler(deps.Emby))
	settingsAPI.GET("/subscription", subscriptionSettingsGetHandler(deps.Monitor))
	settingsAPI.PUT("/subscription", subscriptionSettingsUpdateHandler(deps.Monitor))
	settingsAPI.GET("/sources", sourceListHandler(deps.Sources))
	settingsAPI.PUT("/sources", sourceUpdateHandler(deps.Sources, deps.Library))

	// MPV bridge installer scripts, downloaded from the Outputs settings page.
	protected.GET("/mpv-bridge/install.ps1", noStore(), mpvBridgeScriptHandler(mpvBridgeInstall, "install-mpv-bridge.ps1"))
	protected.GET("/mpv-bridge/uninstall.ps1", noStore(), mpvBridgeScriptHandler(mpvBridgeUninstall, "uninstall-mpv-bridge.ps1"))

	protected.GET("/library/movies", libraryMoviesHandler(deps.Library))
	protected.GET("/library/browse", libraryBrowseHandler(deps.Library))
	protected.GET("/library/filter-options", libraryFilterOptionsHandler(deps.Library))
	protected.GET("/library/top-rated", libraryTopRatedHandler(deps.Library))
	protected.GET("/library/movies/:id", libraryMovieHandler(deps.Library))
	protected.POST("/library/movies/:id/scrape", libraryMovieScrapeHandler(deps.Library))
	protected.GET("/library/movies/:id/previews/:index", libraryPreviewHandler(deps.Library, deps.Metadata))
	subtitlesAPI := protected.Group("/library/movies/:id/subtitles", noStore())
	subtitlesAPI.GET("", subtitleListHandler(deps.Subtitles))
	subtitlesAPI.GET("/search", subtitleSearchHandler(deps.Subtitles))
	subtitlesAPI.POST("/preview", subtitlePreviewHandler(deps.Subtitles))
	subtitlesAPI.POST("", subtitleSaveHandler(deps.Subtitles))
	subtitlesAPI.DELETE("/:subtitleID", subtitleDeleteHandler(deps.Subtitles))
	subtitlesAPI.GET("/:subtitleID/cues", subtitleCuesHandler(deps.Subtitles))
	protected.POST("/library/scan", libraryScanHandler(deps.Library, deps.Emby))
	protected.POST("/library/sync", librarySyncHandler(deps.Library))
	protected.GET("/library/pending", libraryPendingHandler(deps.Library))
	protected.POST("/library/rebuild", libraryRebuildHandler(deps.Library))
	protected.GET("/library/artwork/:key", libraryArtworkHandler(deps.Artwork))

	actorsAPI := protected.Group("/actors", noStore())
	{
		actorsAPI.GET("", actorListHandler(deps.Actors))
		actorsAPI.GET("/:key", actorDetailHandler(deps.Actors, deps.Library))
		actorsAPI.GET("/:key/avatar", actorAvatarHandler(deps.Actors))
		actorsAPI.GET("/:key/image/:index", actorImageHandler(deps.Actors))
	}

	playAPI := protected.Group("/play", noStore())
	playAPI.GET("/files", playFilesHandler(deps.Play))
	playAPI.GET("/start", playStartMovieHandler(deps.Play))
	playAPI.GET("/qualities", playQualitiesHandler(deps.Play))
	playAPI.GET("/:id", playStartHandler(deps.Play))
	playAPI.DELETE("/:id", playReleaseHandler(deps.Play))
	playAPI.GET("/:id/stream/:resource", playStreamHandler(deps.Play))
	playAPI.HEAD("/:id/stream/:resource", playStreamHandler(deps.Play))
	playAPI.POST("/progress", playProgressHandler(deps.Play))

	// Builds the miyabi-mpv:// deep link used by the movie card context menu.
	protected.GET("/mpv-link", noStore(), mpvLinkHandler(deps.Play, deps.Subtitles, deps.STRMToken))

	protected.GET("/history", noStore(), historyHandler(deps.Play))
	protected.POST("/history/remove", historyRemoveHandler(deps.Play))
	protected.DELETE("/history", historyClearHandler(deps.Play))

	protected.GET("/tasks", noStore(), tasksHandler(deps.Tasks))
	protected.POST("/tasks/:id/retry", taskRetryHandler(deps.Tasks))
	protected.PUT("/tasks/library-control", taskLibraryControlHandler(deps.Tasks))
	protected.GET("/tasks/events", taskEventsHandler(deps.Tasks))
	protected.GET("/offline/tasks", noStore(), offlineActivityHandler(deps.Offline))
	protected.POST("/offline/tasks/:id/cancel", offlineControlHandler(deps.Offline, false))
	protected.POST("/offline/tasks/:id/next", offlineControlHandler(deps.Offline, true))

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
	protected.GET("/discover/tags", discoverTagsHandler(deps.Catalogue))
	protected.GET("/discover/movies/:id", discoverMovieHandler(deps.Catalogue, deps.Metadata))
	protected.GET("/discover/movies/:id/magnets", discoverMagnetsHandler(deps.Catalogue))
	protected.POST("/discover/movies/:id/offline", offlineAddHandler(deps.Offline))
	protected.GET("/image", imageHandler(deps.Catalogue))

	panAPI := protected.Group("/pan", noStore())
	panAPI.GET("/accounts", panAccountsHandler(deps.Drive))
	panAPI.GET("/accounts/:accountID", panAccountStatusHandler(deps.Drive))
	panAPI.GET("/accounts/:accountID/files", panAccountFilesHandler(deps.Drive))
	panAPI.PUT("/accounts/:accountID/directory", panAccountSelectDirectoryHandler(deps.Drive))
	panAPI.DELETE("/accounts/:accountID/directory", panAccountClearDirectoryHandler(deps.Drive))
	panAPI.DELETE("/accounts/:accountID", panAccountDisconnectHandler(deps.Drive))
	panAPI.POST("/login", panBeginLoginHandler(deps.Drive))
	panAPI.GET("/login/:id", panLoginStatusHandler(deps.Drive))

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
