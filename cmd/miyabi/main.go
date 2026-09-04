package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ppxb/miyabi"
	"github.com/ppxb/miyabi/internal/api"
	"github.com/ppxb/miyabi/internal/config"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/javdb"
	"github.com/ppxb/miyabi/internal/logging"
	"github.com/ppxb/miyabi/internal/service"
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

	router := api.NewRouter(api.Dependencies{
		Logger:   logger,
		Health:   store,
		Discover: discover,
		Frontend: miyabi.Frontend(),
	})
	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverError := make(chan error, 1)
	go func() {
		logger.Info("HTTP server started", "address", cfg.Listen)
		serverError <- server.ListenAndServe()
	}()

	select {
	case err := <-serverError:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
	}

	logger.Info("HTTP server stopped")
	return nil
}
