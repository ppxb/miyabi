package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ppxb/miyabi"
	"github.com/ppxb/miyabi/internal/api"
	"github.com/ppxb/miyabi/internal/config"
	"github.com/ppxb/miyabi/internal/database"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/javdb"
	"github.com/ppxb/miyabi/internal/logging"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/service"
	"github.com/ppxb/miyabi/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("miyabi stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return err
	}

	logger, err := logging.New(os.Stdout, cfg.LogLevel)
	if err != nil {
		return err
	}
	slog.SetDefault(logger)

	store, err := database.Open(context.Background(), cfg.DataDir)
	if err != nil {
		return err
	}
	defer store.Close()
	discover, err := service.NewDiscoverService(context.Background(), store.Client, javdb.Options{
		Proxy: cfg.Proxy,
	})
	if err != nil {
		return fmt.Errorf("initialize discovery service: %w", err)
	}
	defer discover.Close()
	drive, err := service.NewPanService(context.Background(), store.Client, pan.Options{
		Proxy: cfg.Proxy,
	})
	if err != nil {
		return fmt.Errorf("initialize pan service: %w", err)
	}
	defer drive.Close()
	tasks := service.NewTaskService(store.Client)
	offline := service.NewOfflineService(store.Client, discover, drive, tasks)
	images, err := mediaimage.NewCache(cfg.DataDir)
	if err != nil {
		return err
	}
	library := service.NewLibraryService(store.Client, drive, tasks, images)
	play := service.NewPlayService(library)
	defer play.Close()
	scrape := service.NewScrapeService(library, discover, images)
	// Keep scans, metadata writes and directory sidecars ordered.
	pool := worker.NewPool(tasks, map[string]worker.Handler{
		"scan": library.Scan, "scrape": scrape.Scrape, "cover": scrape.Cover,
	}, 1, logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	router := api.NewRouter(api.Dependencies{
		Logger:   logger,
		Health:   store,
		Discover: discover,
		Pan:      drive,
		Offline:  offline,
		Library:  library,
		Play:     play,
		Tasks:    tasks,
		Artwork:  scrape,
		Frontend: miyabi.Frontend(),
	})
	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	workerDone := make(chan struct{})
	poolDone := make(chan struct{})
	poolError := make(chan error, 1)
	go func() {
		defer close(poolDone)
		poolError <- pool.Run(ctx)
	}()
	go func() {
		defer close(workerDone)
		worker.RunOffline(ctx, offline, logger)
	}()
	defer func() {
		stop()
		<-workerDone
		<-poolDone
	}()

	serverError := make(chan error, 1)
	go func() {
		logger.Info("HTTP server started", "address", cfg.Listen)
		serverError <- server.ListenAndServe()
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
	stop()
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		return errors.Join(runError, fmt.Errorf("shutdown HTTP server: %w", err))
	}

	logger.Info("HTTP server stopped")
	return runError
}
