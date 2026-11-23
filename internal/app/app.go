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
	"github.com/ppxb/miyabi/internal/actors"
	"github.com/ppxb/miyabi/internal/api"
	"github.com/ppxb/miyabi/internal/catalogue"
	"github.com/ppxb/miyabi/internal/config"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/emby"
	"github.com/ppxb/miyabi/internal/export"
	"github.com/ppxb/miyabi/internal/freejavbt"
	"github.com/ppxb/miyabi/internal/gfriends"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/javbus"
	"github.com/ppxb/miyabi/internal/library"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/maintenance"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/metadata/providers"
	"github.com/ppxb/miyabi/internal/monitor"
	"github.com/ppxb/miyabi/internal/network"
	"github.com/ppxb/miyabi/internal/netx"
	"github.com/ppxb/miyabi/internal/offline"
	"github.com/ppxb/miyabi/internal/playback"
	"github.com/ppxb/miyabi/internal/projectjav"
	"github.com/ppxb/miyabi/internal/source"
	"github.com/ppxb/miyabi/internal/strm"
	"github.com/ppxb/miyabi/internal/subtitle"
	"github.com/ppxb/miyabi/internal/sukebei"
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
	play      *playback.Service
	catalogue *catalogue.Service
	metadata  *metadata.Service
	embySvc   *emby.Service
	library   *library.Service
	export    *export.Manager
}

// New initializes all services, database connections, and registers task handlers.
// cfg must be initialized by config.Load or supplied with explicit configuration.
func New(cfg *config.Config, logger *slog.Logger) (_ *App, err error) {
	ctx := context.Background()
	var cleanup []func()
	defer func() {
		if err != nil {
			for i := len(cleanup) - 1; i >= 0; i-- {
				cleanup[i]()
			}
		}
	}()

	store, err := database.Open(ctx, cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	cleanup = append(cleanup, func() { _ = store.Close() })

	networkSvc, err := network.New(ctx, store.Client)
	if err != nil {
		return nil, fmt.Errorf("initialize network service: %w", err)
	}

	taskRegistry := tasks.NewRegistry()
	taskSvc := tasks.NewService(store.Client, taskRegistry)
	driveSvc, err := drive.New(ctx, store.Client)
	if err != nil {
		return nil, fmt.Errorf("initialize drive service: %w", err)
	}
	cleanup = append(cleanup, driveSvc.Close)
	sourceSvc := source.New(store.Client, driveSvc)

	images, err := mediaimage.NewCache(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("initialize images cache: %w", err)
	}

	exportMgr := export.NewManager(export.Config{
		EmbyDir: cfg.EmbyDir, PublicURL: cfg.PublicURL, STRMToken: cfg.STRMToken,
	})
	playSvc := playback.New(store.Client, driveSvc, 8*time.Hour)
	libSvc := library.New(store.Client, driveSvc, taskSvc, images, library.Options{
		ExportManager: exportMgr,
		Sources:       sourceSvc.LibrarySources,
		LocalSources: func(ctx context.Context) ([]library.LocalScanTarget, error) {
			targets, err := sourceSvc.LocalTargets(ctx)
			if err != nil {
				return nil, err
			}
			result := make([]library.LocalScanTarget, 0, len(targets))
			for _, target := range targets {
				result = append(result, library.LocalScanTarget{Path: target.Path, MetadataOnly: target.MetadataOnly})
			}
			return result, nil
		},
	})
	javbusClient, err := javbus.New(javbus.Options{Proxy: networkSvc.ProxyManager()})
	if err != nil {
		return nil, err
	}
	sukebeiClient, err := sukebei.New(sukebei.Options{Proxy: networkSvc.ProxyManager()})
	if err != nil {
		return nil, fmt.Errorf("initialize Sukebei magnet source: %w", err)
	}
	freejavbtClient, err := freejavbt.New(freejavbt.Options{Proxy: networkSvc.ProxyManager()})
	if err != nil {
		return nil, fmt.Errorf("initialize freejavbt source: %w", err)
	}
	projectjavClient, err := projectjav.New(projectjav.Options{Proxy: networkSvc.ProxyManager()})
	if err != nil {
		return nil, fmt.Errorf("initialize ProjectJav magnet source: %w", err)
	}
	// catalogue owns the magnet clients and metadata owns its providers: each
	// service's Close releases what it was given, so they are not registered
	// separately here. freejavbt is shared by both; its Close is idempotent.
	catalogueSvc, err := catalogue.New(ctx, store.Client, networkSvc.ProxyManager(), libSvc, javbusClient, sukebeiClient, freejavbtClient, projectjavClient)
	if err != nil {
		return nil, fmt.Errorf("initialize catalogue service: %w", err)
	}
	cleanup = append(cleanup, catalogueSvc.Close)
	proxy := networkSvc.ProxyManager()
	metadataSvc, err := metadata.New(ctx, store.Client,
		providers.NewFANZA(proxy),
		providers.NewFC2(proxy),
		providers.NewFC2DB(proxy),
		providers.NewHEYZO(proxy),
		providers.NewPacopacomama(proxy),
		providers.NewJavDB(catalogueSvc),
		providers.NewJavBus(javbusClient, proxy),
		providers.NewR18(proxy),
		providers.NewOnePondo(proxy),
		providers.NewCodeAV(proxy),
		providers.NewJav321(proxy),
		providers.NewTokyoHot(proxy),
		providers.NewHeydouga(proxy),
		providers.NewCaribbeancom(proxy),
		providers.NewCaribbeancomPremium(proxy),
		providers.NewC0930(proxy),
		providers.NewH0930(proxy),
		providers.NewH4610(proxy),
		providers.NewTenMusume(proxy),
		providers.NewMuraMura(proxy),
		providers.NewDahlia(proxy),
		providers.NewDuga(proxy),
		freejavbtClient,
	)
	if err != nil {
		return nil, err
	}
	cleanup = append(cleanup, metadataSvc.Close)

	offlineSvc := offline.New(store.Client, catalogueSvc, driveSvc, taskSvc, libSvc, metadataSvc, offlineSubmitTimeout)
	monitorSvc := monitor.New(store.Client, catalogueSvc, offlineSvc, taskSvc)
	gfriendsClient := gfriends.New(cfg.DataDir, &http.Client{
		Timeout:   30 * time.Second,
		Transport: netx.NewTransport(networkSvc.ProxyManager()),
	})
	actorsSvc := actors.New(
		store.Client,
		gfriendsClient,
		cfg.DataDir,
		netx.NewSafeDownloadClient(networkSvc.ProxyManager(), 30*time.Second),
		actors.NewAVLeague(networkSvc.ProxyManager()),
	)
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
		Media:             metadataSvc,
		ExportManager:     exportMgr,
		ScheduleLocalScan: libSvc.ScheduleLocalScan,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize emby service: %w", err)
	}
	cleanup = append(cleanup, embySvc.Close)
	// This is the only cycle: library -> Emby -> catalogue -> library.
	// Bind it before starting task pools or accepting requests.
	libSvc.SetMediaNotifier(embySvc)
	subtitleSvc := subtitle.NewService(store.Client, subtitle.NewFinder(networkSvc.ProxyManager()),
		filepath.Join(cfg.DataDir, "subtitles"), func() string { return exportMgr.Config().EmbyDir }, embySvc)
	scrapeSvc := scrape.New(store.Client, driveSvc, metadataSvc, images, taskSvc, scrape.Dependencies{
		ExportManager: exportMgr, MediaNotifier: embySvc, Subtitles: subtitleSvc, Merger: actorsSvc,
	})
	maintenanceSvc, err := maintenance.New(cfg.DataDir, store.Client, images, scrapeSvc)
	if err != nil {
		return nil, fmt.Errorf("initialize maintenance service: %w", err)
	}

	taskRegistry.Register(tasks.NewHandler(tasks.KindScan, libSvc.Scan, libSvc.Finished).WithRetry(domain.RetryDelay))
	taskRegistry.Register(tasks.NewHandler(tasks.KindScrape, scrapeSvc.Scrape, scrapeSvc.Finished).WithRetry(domain.RetryDelay))
	taskRegistry.Register(tasks.NewHandler(tasks.KindSubscriptionBatch, monitorSvc.BatchHandler, monitorSvc.BatchFinished))

	pools := newTaskPools(taskSvc, logger)

	router := api.NewRouter(api.Dependencies{
		Logger:         logger,
		Access:         api.NewAccessGateService(cfg.AccessPassword, cfg.JWTSecret),
		Catalogue:      catalogueSvc,
		Metadata:       metadataSvc,
		Drive:          driveSvc,
		Offline:        offlineSvc,
		Monitor:        monitorSvc,
		Library:        libSvc,
		Play:           playSvc,
		STRM:           strm.New(store.Client, driveSvc),
		Tasks:          &taskViews{Service: taskSvc, database: store.Client, library: libSvc, monitor: monitorSvc},
		Artwork:        scrapeSvc,
		Maintenance:    maintenanceSvc,
		Network:        networkSvc,
		Emby:           embySvc,
		Sources:        sourceSvc,
		Actors:         actorsSvc,
		Subtitles:      subtitleSvc,
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
		play:      playSvc,
		catalogue: catalogueSvc,
		metadata:  metadataSvc,
		embySvc:   embySvc,
		library:   libSvc,
		export:    exportMgr,
	}, nil
}

// start runs the startup side effects that New deliberately leaves out: the
// account token refresh loop, the startup scans and the STRM rewrite. Sources
// only get an incremental sync: new files are still discovered, but existing
// movies are not all re-scraped on every start.
func (a *App) start(ctx context.Context) {
	a.driveSvc.StartAccountRefresh(ctx)
	if err := a.library.ScheduleLocalScan(ctx); err != nil {
		a.logger.ErrorContext(ctx, "failed to queue startup Emby directory scan", "error", err)
	}
	if _, err := a.library.StartIncrementalSync(ctx); err != nil {
		a.logger.ErrorContext(ctx, "failed to queue startup source sync", "error", err)
	}
	if exportCfg := a.export.Config(); exportCfg.PublicURL != "" && exportCfg.EmbyDir != "" {
		a.embySvc.StartStartupSTRMRewrite()
	}
}

// Run runs the startup side effects, starts the task pools, the periodic
// workers and the HTTP server, then blocks until ctx is cancelled or one of
// them fails. Every exit path shuts the server down gracefully and waits for
// the workers before returning.
func (a *App) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	a.server.BaseContext = func(net.Listener) context.Context { return ctx }
	a.start(ctx)

	var workers sync.WaitGroup
	poolError := make(chan error, len(a.pools))
	for _, pool := range a.pools {
		workers.Go(func() {
			poolError <- pool.Run(ctx)
		})
	}
	workers.Go(func() {
		tasks.RunPeriodic(ctx, a.logger, "sync 115 offline tasks", offlineSyncInterval, nil, a.offline.Sync)
	})
	workers.Go(func() {
		tasks.RunPeriodic(ctx, a.logger, "monitor", monitorCheckInterval, a.monitors.Pending(), a.monitors.Check)
	})

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
	a.embySvc.Close()
	a.metadata.Close()
	a.catalogue.Close()
	a.play.Close()
	a.driveSvc.Close()
	return a.store.Close()
}
