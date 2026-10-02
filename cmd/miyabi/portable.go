package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ppxb/miyabi/internal/app"
	"github.com/ppxb/miyabi/internal/config"
)

// logFileLimit rotates the portable log once it grows past this size, so a
// long-running window-less build cannot fill the disk.
const logFileLimit = 8 << 20

// browserStartupTimeout bounds how long a double-click waits for the server
// before giving up on opening the browser.
const browserStartupTimeout = 30 * time.Second

// newLogger writes logs to stdout and, for a portable build without a console,
// to <dataDir>/miyabi.log as well. Callers must close the returned log before
// returning, after recording the failure that stops the process.
func newLogger(cfg config.Config, portable bool) (*slog.Logger, func()) {
	writer := io.Writer(os.Stdout)
	closeLog := func() {}
	if portable {
		file, err := openLogFile(cfg.DataDir)
		if err != nil {
			slog.Warn("cannot write the portable log file", "directory", cfg.DataDir, "error", err)
		} else {
			// The file comes first: a window-less build has no console, and a
			// failing stdout write must not swallow the log line.
			writer = io.MultiWriter(file, os.Stdout)
			closeLog = func() { _ = file.Close() }
		}
	}
	return slog.New(slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: cfg.LogLevel})), closeLog
}

func openLogFile(dataDir string) (*os.File, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "miyabi.log")
	if info, err := os.Stat(path); err == nil && info.Size() > logFileLimit {
		_ = os.Remove(path + ".1")
		_ = os.Rename(path, path+".1")
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}

// openBrowser opens the local UI in the default browser.
func openBrowser(listen string) {
	url, err := app.LocalURL(listen)
	if err != nil {
		slog.Warn("cannot open the browser for an invalid listen address", "listen", listen, "error", err)
		return
	}
	name, args := browserCommand(runtime.GOOS, url)
	if err := exec.Command(name, args...).Start(); err != nil {
		slog.Warn("cannot open the browser", "url", url, "error", err)
	}
}

// openBrowserWhenReady waits for the health endpoint so a double-click never
// lands on a dead page, then opens the local UI. It gives up as soon as the
// server stops, so a startup failure is reported without waiting for the poll.
func openBrowserWhenReady(ctx context.Context, listen string, stopped <-chan struct{}) {
	deadline := time.Now().Add(browserStartupTimeout)
	for {
		if app.CheckHealth(listen) == nil {
			openBrowser(listen)
			return
		}
		if time.Now().After(deadline) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-stopped:
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// reportFatal surfaces a startup failure to a user who double-clicked a
// window-less build, where a log line alone would be invisible.
func reportFatal(portable bool, err error) {
	if !portable {
		return
	}
	messageBox("Miyabi 启动失败", fmt.Sprintf("%v\n\n详细日志见数据目录下的 miyabi.log。", err))
}

// browserCommand returns the platform command that opens a URL.
func browserCommand(goos, url string) (string, []string) {
	switch goos {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		return "open", []string{url}
	default:
		return "xdg-open", []string{url}
	}
}
