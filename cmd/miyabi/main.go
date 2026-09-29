package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ppxb/miyabi/internal/app"
	"github.com/ppxb/miyabi/internal/config"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("miyabi stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	healthcheck := len(args) == 1 && args[0] == "healthcheck"
	if len(args) > 0 && !healthcheck {
		return errors.New("usage: miyabi [healthcheck]; configure the application with MIYABI_* environment variables")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if healthcheck {
		return checkHealth(cfg.Listen)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	application, err := app.New(&cfg, logger)
	if err != nil {
		return err
	}
	defer application.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return application.Run(ctx)
}
