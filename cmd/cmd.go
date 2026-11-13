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
	"github.com/ppxb/miyabi/internal/tray"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		slog.Error("miyabi stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) > 0 {
		return errors.New("usage: miyabi; configure the application with MIYABI_* environment variables")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	application, err := app.New(&cfg, logger)
	if err != nil {
		return err
	}
	defer application.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	return tray.Run(ctx, cancel, tray.Options{
		Title:   "Miyabi",
		URL:     cfg.LocalURL(),
		DataDir: cfg.DataDir,
	}, application.Run)
}
