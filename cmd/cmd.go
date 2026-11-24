package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ppxb/miyabi/internal/app"
	"github.com/ppxb/miyabi/internal/config"
	"github.com/ppxb/miyabi/internal/logging"
	"github.com/ppxb/miyabi/internal/tray"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if run(ctx, os.Args[1:]) != nil {
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) (err error) {
	if len(args) > 0 {
		return errors.New("usage: miyabi; configure the application with MIYABI_* environment variables")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Claim the single-instance slot before opening the database or log so a
	// second launch never touches shared state; it just focuses the first UI.
	owner, release, err := tray.AcquireInstance()
	if err != nil {
		return err
	}
	if !owner {
		tray.Open(cfg.LocalURL())
		return nil
	}
	defer release()

	logFile, err := logging.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer logFile.Close()

	logger := slog.New(slog.NewJSONHandler(io.MultiWriter(logFile, os.Stdout), &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)
	// Log the fatal error while the log file is still open: defers run LIFO, so
	// this registered-later defer fires before the earlier logFile.Close.
	defer func() {
		if err != nil {
			slog.Error("miyabi stopped", "error", err)
		}
	}()

	application, err := app.New(&cfg, logger)
	if err != nil {
		return err
	}
	defer application.Close()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	return tray.Run(runCtx, cancel, tray.Options{
		Title:   "Miyabi",
		URL:     cfg.LocalURL(),
		DataDir: cfg.DataDir,
	}, application.Run)
}
