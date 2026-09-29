package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/ppxb/miyabi"
	"github.com/ppxb/miyabi/internal/api"
	"github.com/ppxb/miyabi/internal/catalogue"
	"github.com/ppxb/miyabi/internal/config"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/emby"
	"github.com/ppxb/miyabi/internal/export"
	"github.com/ppxb/miyabi/internal/gfriends"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/library"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/maintenance"
	"github.com/ppxb/miyabi/internal/monitor"
	"github.com/ppxb/miyabi/internal/network"
	"github.com/ppxb/miyabi/internal/netx"
	"github.com/ppxb/miyabi/internal/offline"
	"github.com/ppxb/miyabi/internal/strm"
	"github.com/ppxb/miyabi/internal/subtitle"
	"github.com/ppxb/miyabi/internal/tasks"
)

const (
	offlineSyncInterval  = 30 * time.Second
	monitorCheckInterval = 5 * time.Minute
	// Bound a remote offline submission after detaching from its caller.
	offlineSubmitTimeout = 2 * time.Minute
)

// App is the composition root assembling services, background workers, and the HTTP server.
type App struct {
	cfg       *config.Config
	logger    *slog.Logger
	store     *database.Store
	server    *http.Server
	pools     []*tasks.Pool
	offline   *offline.Service
	monitors  *monitor.Service
	driveSvc  *drive.Drive
	catalogue *catalogue.Service
	scrape    *scrape.Service
	embySvc   *emby.Service
}

// New initializes all services, database connections, and registers task handlers.
func New(cfg *config.Config, logger *slog.Logger) (*App, error) {
	if cfg.EmbyDir == "" {
		cfg.EmbyDir = filepath.Join(cfg.DataDir, "emby")
	}
	ctx := context.Background()
	store, err := database.Open(ctx, cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	networkSvc, err := network.New(ctx, store.Client)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("initialize network service: %w", err)
	}

	taskRegistry := tasks.NewRegistry()
	taskSvc := tasks.NewService(store.Client, taskRegistry)
	driveSvc, err := drive.New(ctx, store.Client)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("initialize drive service: %w", err)
	}

	images, err := mediaimage.NewCache(cfg.DataDir)
	if err != nil {
		driveSvc.Close()
		_ = store.Close()
		return nil, fmt.Errorf("initialize images cache: %w", err)
	}

	exportMgr := export.NewManager(export.Config{
		EmbyDir: cfg.EmbyDir, PublicURL: cfg.PublicURL, STRMToken: cfg.STRMToken,
	})
	libSvc := library.New(store.Client, driveSvc, taskSvc, images, library.Options{ExportManager: exportMgr})
	catalogueSvc, err := catalogue.New(ctx, store.Client, networkSvc.ProxyManager(), libSvc)
	if err != nil {
		driveSvc.Close()
		_ = store.Close()
		return nil, fmt.Errorf("initialize catalogue service: %w", err)
	}

	offlineSvc := offline.New(store.Client, catalogueSvc, driveSvc, taskSvc, libSvc, offlineSubmitTimeout)
	monitorSvc := monitor.New(store.Client, catalogueSvc, offlineSvc, taskSvc)
	subtitleSvc := subtitle.NewService(store.Client, subtitle.NewFinder(networkSvc.ProxyManager()))
	gfriendsClient := gfriends.New(cfg.DataDir, &http.Client{
		Timeout:   30 * time.Second,
		Transport: netx.NewTransport(networkSvc.ProxyManager()),
	})
	syncActors := cfg.EmbySyncActors
	embySvc, err := emby.NewService(ctx, store.Client, emby.Config{
		Enabled:    cfg.EmbyEnabled,
		ServerURL:  cfg.EmbyServerURL,
		APIKey:     cfg.EmbyAPIKey,
		MediaPath:  cfg.EmbyMediaPath,
		LocalDir:   cfg.EmbyDir,
		SyncActors: &syncActors,
		PublicURL:  cfg.PublicURL,
	}, emby.Dependencies{
		GFriends:          gfriendsClient,
		Media:             catalogueSvc,
		ExportManager:     exportMgr,
		ScheduleLocalScan: libSvc.ScheduleLocalScan,
	})
	if err != nil {
		catalogueSvc.Close()
		driveSvc.Close()
		_ = store.Close()
		return nil, fmt.Errorf("initialize emby service: %w", err)
	}
	// This is the only cycle: library -> Emby -> catalogue -> library.
	// Bind it before starting task pools or accepting requests.
	libSvc.SetMediaNotifier(embySvc)
	scrapeSvc := scrape.New(store.Client, driveSvc, catalogueSvc, images, taskSvc, scrape.Dependencies{
		ExportManager: exportMgr, MediaNotifier: embySvc, Subtitles: subtitleSvc,
	})
	maintenanceSvc, err := maintenance.New(cfg.DataDir, store.Client, images, scrapeSvc)
	if err != nil {
		scrapeSvc.Close()
		embySvc.Close()
		catalogueSvc.Close()
		driveSvc.Close()
		_ = store.Close()
		return nil, fmt.Errorf("initialize maintenance service: %w", err)
	}

	if err := libSvc.ScheduleLocalScan(ctx); err != nil {
		logger.ErrorContext(ctx, "failed to queue startup Emby directory scan", "error", err)
	}
	if exportCfg := exportMgr.Config(); exportCfg.PublicURL != "" && exportCfg.EmbyDir != "" {
		embySvc.StartStartupSTRMRewrite()
	}

	taskRegistry.Register(tasks.NewHandler(tasks.KindScan, libSvc.Scan, libSvc.Finished))
	taskRegistry.Register(tasks.NewHandler(tasks.KindScrape, scrapeSvc.Scrape, scrapeSvc.Finished))
	taskRegistry.Register(tasks.NewHandler(tasks.KindCover, scrapeSvc.Cover, scrapeSvc.Finished))
	taskRegistry.Register(tasks.NewHandler(tasks.KindSubscriptionBatch, monitorSvc.BatchHandler, monitorSvc.BatchFinished))

	pools := newTaskPools(taskSvc, logger)

	router := api.NewRouter(api.Dependencies{
		Logger:         logger,
		Health:         store,
		Access:         api.NewAccessGateService(cfg.AccessPassword, cfg.JWTSecret),
		Catalogue:      catalogueSvc,
		Drive:          driveSvc,
		Offline:        offlineSvc,
		Monitor:        monitorSvc,
		Library:        libSvc,
		STRM:           strm.New(store.Client, driveSvc),
		Tasks:          &taskViews{Service: taskSvc, database: store.Client, library: libSvc, monitor: monitorSvc},
		Artwork:        scrapeSvc,
		Maintenance:    maintenanceSvc,
		Network:        networkSvc,
		Emby:           embySvc,
		Frontend:       miyabi.Frontend(),
		STRMToken:      cfg.STRMToken,
		TrustedProxies: cfg.TrustedProxies,
	})

	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return &App{
		cfg:       cfg,
		logger:    logger,
		store:     store,
		server:    server,
		pools:     pools,
		offline:   offlineSvc,
		monitors:  monitorSvc,
		driveSvc:  driveSvc,
		catalogue: catalogueSvc,
		scrape:    scrapeSvc,
		embySvc:   embySvc,
	}, nil
}

// Run starts the task pools, the periodic workers and the HTTP server, then
// blocks until ctx is cancelled or one of them fails. Every exit path shuts
// the server down gracefully and waits for the workers before returning.
func (a *App) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	a.server.BaseContext = func(net.Listener) context.Context { return ctx }

	var workers sync.WaitGroup
	poolError := make(chan error, len(a.pools))
	workers.Add(len(a.pools) + 2)
	for _, pool := range a.pools {
		go func() {
			defer workers.Done()
			poolError <- pool.Run(ctx)
		}()
	}
	go func() {
		defer workers.Done()
		tasks.RunPeriodic(ctx, a.logger, "sync 115 offline tasks", offlineSyncInterval, nil, a.offline.Sync)
	}()
	go func() {
		defer workers.Done()
		tasks.RunPeriodic(ctx, a.logger, "monitor", monitorCheckInterval, a.monitors.Pending(), a.monitors.Check)
	}()

	serverError := make(chan error, 1)
	go func() {
		a.logger.Info("HTTP server started", "address", a.cfg.Listen)
		serverError <- a.server.ListenAndServe()
	}()

	var runError error
	select {
	case err := <-serverError:
		if !errors.Is(err, http.ErrServerClosed) {
			runError = fmt.Errorf("serve HTTP: %w", err)
		}
	case err := <-poolError:
		if err != nil {
			runError = fmt.Errorf("run task pool: %w", err)
		}
	case <-ctx.Done():
	}

	cancel()
	shutdownContext, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	if err := a.server.Shutdown(shutdownContext); err != nil {
		runError = errors.Join(runError, fmt.Errorf("shutdown HTTP server: %w", err))
	}
	workers.Wait()
	a.logger.Info("HTTP server stopped")
	return runError
}

// Close releases resources held by the application.
func (a *App) Close() error {
	if a.embySvc != nil {
		a.embySvc.Close()
	}
	if a.scrape != nil {
		a.scrape.Close()
	}
	if a.catalogue != nil {
		a.catalogue.Close()
	}
	if a.driveSvc != nil {
		a.driveSvc.Close()
	}
	if a.store != nil {
		return a.store.Close()
	}
	return nil
}

// CheckHealth probes the health of a running server given its listen address.
func CheckHealth(listen string) error {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("parse health check address: %w", err)
	}
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	client := &http.Client{
		Timeout: 4 * time.Second,
		// A local probe must not use proxy environment variables.
		Transport: &http.Transport{DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Get("http://" + net.JoinHostPort(host, port) + "/api/health")
	if err != nil {
		return fmt.Errorf("check health: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned HTTP %d", response.StatusCode)
	}
	return nil
}
