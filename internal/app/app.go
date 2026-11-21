package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
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
	scrape    *scrape.Service
	metadata  *metadata.Service
	embySvc   *emby.Service
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
	sourceSvc := source.New(store.Client)
	driveSvc, err := drive.New(ctx, store.Client)
	if err != nil {
		return nil, fmt.Errorf("initialize drive service: %w", err)
	}
	cleanup = append(cleanup, driveSvc.Close)
	driveSvc.StartAccountRefresh(ctx)

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
	sourceSvc.SetMountProvider(driveSvc)
	sourceSvc.SetScanHandler(func(ctx context.Context, src domain.LibrarySource) error {
		_, err := libSvc.EnqueueSyncMode(ctx, src, false)
		return err
	})
	sourceSvc.SetLocalScanHandler(func(ctx context.Context, root string, metadataOnly bool) error {
		_, err := libSvc.StartLocalSourceScanModeIncremental(ctx, root, metadataOnly)
		return err
	})
	javbusClient, err := javbus.New(javbus.Options{Proxy: networkSvc.ProxyManager()})
	if err != nil {
		return nil, err
	}
	cleanup = append(cleanup, javbusClient.Close)
	sukebeiClient, err := sukebei.New(sukebei.Options{Proxy: networkSvc.ProxyManager()})
	if err != nil {
		return nil, fmt.Errorf("initialize Sukebei magnet source: %w", err)
	}
	cleanup = append(cleanup, sukebeiClient.Close)
	freejavbtClient, err := freejavbt.New(freejavbt.Options{Proxy: networkSvc.ProxyManager()})
	if err != nil {
		return nil, fmt.Errorf("initialize freejavbt source: %w", err)
	}
	cleanup = append(cleanup, freejavbtClient.Close)
	projectjavClient, err := projectjav.New(projectjav.Options{Proxy: networkSvc.ProxyManager()})
	if err != nil {
		return nil, fmt.Errorf("initialize ProjectJav magnet source: %w", err)
	}
	cleanup = append(cleanup, projectjavClient.Close)
	catalogueSvc, err := catalogue.New(ctx, store.Client, networkSvc.ProxyManager(), libSvc, javbusClient, sukebeiClient, freejavbtClient, projectjavClient)
	if err != nil {
		return nil, fmt.Errorf("initialize catalogue service: %w", err)
	}
	cleanup = append(cleanup, catalogueSvc.Close)
	fc2Source := providers.NewFC2(networkSvc.ProxyManager())
	fc2dbSource := providers.NewFC2DB(networkSvc.ProxyManager())
	fanzaSource := providers.NewFANZA(networkSvc.ProxyManager())
	heyzoSource := providers.NewHEYZO(networkSvc.ProxyManager())
	pacopacomamaSource := providers.NewPacopacomama(networkSvc.ProxyManager())
	jav321Source := providers.NewJav321(networkSvc.ProxyManager())
	tokyohotSource := providers.NewTokyoHot(networkSvc.ProxyManager())
	heydougaSource := providers.NewHeydouga(networkSvc.ProxyManager())
	caribbeancomSource := providers.NewCaribbeancom(networkSvc.ProxyManager())
	caribbeancomprSource := providers.NewCaribbeancomPremium(networkSvc.ProxyManager())
	c0930Source := providers.NewC0930(networkSvc.ProxyManager())
	h0930Source := providers.NewH0930(networkSvc.ProxyManager())
	h4610Source := providers.NewH4610(networkSvc.ProxyManager())
	tenmusumeSource := providers.NewTenMusume(networkSvc.ProxyManager())
	muramuraSource := providers.NewMuraMura(networkSvc.ProxyManager())
	dahliaSource := providers.NewDahlia(networkSvc.ProxyManager())
	dugaSource := providers.NewDuga(networkSvc.ProxyManager())
	cleanup = append(cleanup,
		fc2Source.Close, fc2dbSource.Close, fanzaSource.Close, heyzoSource.Close,
		pacopacomamaSource.Close, jav321Source.Close, tokyohotSource.Close, heydougaSource.Close,
		caribbeancomSource.Close, caribbeancomprSource.Close, c0930Source.Close, h0930Source.Close,
		h4610Source.Close, tenmusumeSource.Close, muramuraSource.Close, dahliaSource.Close,
		dugaSource.Close,
	)
	metadataSvc, err := metadata.New(ctx, store.Client, fanzaSource, fc2Source, fc2dbSource, heyzoSource, pacopacomamaSource,
		providers.NewJavDB(catalogueSvc), providers.NewJavBus(javbusClient, networkSvc.ProxyManager()),
		providers.NewR18(networkSvc.ProxyManager()),
		providers.NewOnePondo(networkSvc.ProxyManager()),
		providers.NewCodeAV(networkSvc.ProxyManager()),
		jav321Source, tokyohotSource, heydougaSource,
		caribbeancomSource, caribbeancomprSource, c0930Source, h0930Source, h4610Source, tenmusumeSource, muramuraSource,
		dahliaSource, dugaSource, freejavbtClient)
	if err != nil {
		return nil, err
	}
	cleanup = append(cleanup, metadataSvc.Close)

	offlineSvc := offline.New(store.Client, catalogueSvc, driveSvc, taskSvc, libSvc, offlineSubmitTimeout)
	offlineSvc.SetMetadata(metadataSvc)
	monitorSvc := monitor.New(store.Client, catalogueSvc, offlineSvc, taskSvc)
	subtitleSvc := subtitle.NewService(store.Client, subtitle.NewFinder(networkSvc.ProxyManager()))
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
	scrapeSvc := scrape.New(store.Client, driveSvc, metadataSvc, images, taskSvc, scrape.Dependencies{
		ExportManager: exportMgr, MediaNotifier: embySvc, Subtitles: subtitleSvc, Merger: actorsSvc,
	})
	cleanup = append(cleanup, scrapeSvc.Close)
	maintenanceSvc, err := maintenance.New(cfg.DataDir, store.Client, images, scrapeSvc)
	if err != nil {
		return nil, fmt.Errorf("initialize maintenance service: %w", err)
	}

	if err := libSvc.ScheduleLocalScan(ctx); err != nil {
		logger.ErrorContext(ctx, "failed to queue startup Emby directory scan", "error", err)
	}
	if targets, err := sourceSvc.LocalTargets(ctx); err != nil {
		logger.ErrorContext(ctx, "failed to load local sources", "error", err)
	} else {
		for _, target := range targets {
			if _, err := libSvc.StartLocalSourceScanMode(ctx, target.Path, target.MetadataOnly); err != nil {
				logger.ErrorContext(ctx, "failed to queue local source scan", "path", target.Path, "error", err)
			}
		}
	}
	if mounts, err := sourceSvc.LibrarySources(ctx); err != nil {
		logger.ErrorContext(ctx, "failed to load cloud sources", "error", err)
	} else {
		for _, source := range mounts {
			if _, err := libSvc.EnqueueScan(ctx, source); err != nil {
				logger.ErrorContext(ctx, "failed to queue cloud source scan", "error", err)
			}
		}
	}
	if exportCfg := exportMgr.Config(); exportCfg.PublicURL != "" && exportCfg.EmbyDir != "" {
		embySvc.StartStartupSTRMRewrite()
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
		scrape:    scrapeSvc,
		metadata:  metadataSvc,
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
	a.scrape.Close()
	a.metadata.Close()
	a.catalogue.Close()
	a.play.Close()
	a.driveSvc.Close()
	return a.store.Close()
}
