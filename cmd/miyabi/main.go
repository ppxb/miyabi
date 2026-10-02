package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/ppxb/miyabi/internal/app"
	"github.com/ppxb/miyabi/internal/config"
)

func main() {
	// run reports its own failure: by then the portable log file is the only
	// record a window-less build leaves behind, and it must still be open.
	if err := run(os.Args[1:]); err != nil {
		os.Exit(1)
	}
}

func run(args []string) error {
	healthcheck := len(args) == 1 && args[0] == "healthcheck"
	if len(args) > 0 && !healthcheck {
		return fail(false, errors.New("usage: miyabi [healthcheck]; configure the application with MIYABI_* environment variables"))
	}
	cfg, portable, err := loadConfig()
	if err != nil {
		return fail(portable, err)
	}
	if healthcheck {
		return fail(false, checkHealth(cfg.Listen))
	}

	logger, closeLog := newLogger(cfg, portable)
	defer closeLog()
	slog.SetDefault(logger)

	// A second double-click must reach the running instance instead of failing
	// on a port that is already bound.
	if portable && checkHealth(cfg.Listen) == nil {
		openBrowser(cfg.Listen)
		return nil
	}

	application, err := app.New(&cfg, logger)
	if err != nil {
		return fail(portable, err)
	}
	defer application.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runError := make(chan error, 1)
	stopped := make(chan struct{})
	go func() {
		runError <- application.Run(ctx)
		close(stopped)
	}()
	if portable {
		openBrowserWhenReady(ctx, cfg.Listen, stopped)
	}
	if err := <-runError; err != nil {
		return fail(portable, err)
	}
	return nil
}

// fail records a fatal error and makes it visible to a user who double-clicked
// a window-less build, where a log line alone would be invisible.
func fail(portable bool, err error) error {
	slog.Error("miyabi stopped", "error", err)
	reportFatal(portable, err)
	return err
}

// loadConfig starts a double-clicked Windows build from the configuration file
// next to its executable, and every other process from the environment alone.
func loadConfig() (config.Config, bool, error) {
	exeDir, err := config.ExecutableDir()
	if err != nil {
		slog.Warn("cannot locate the executable; using environment-only configuration", "error", err)
		exeDir = ""
	}
	if !config.Portable(runtime.GOOS, exeDir, os.Environ()) {
		cfg, loadErr := config.Load()
		return cfg, false, loadErr
	}
	cfg, loadErr := config.LoadPortable(exeDir)
	return cfg, true, loadErr
}
