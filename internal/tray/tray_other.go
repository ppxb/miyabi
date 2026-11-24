//go:build !windows

package tray

import (
	"context"
	"os/exec"
	"runtime"
)

// AcquireInstance always succeeds on platforms without a tray.
func AcquireInstance() (owner bool, release func(), err error) {
	return true, func() {}, nil
}

// Open opens target with the platform default handler (best-effort).
func Open(target string) {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("open", target)
	} else {
		cmd = exec.Command("xdg-open", target)
	}
	_ = cmd.Start()
}

// Run falls back to a plain foreground server on platforms without a tray.
func Run(ctx context.Context, cancel context.CancelFunc, _ Options, serve ServeFunc) error {
	defer cancel()
	return serve(ctx)
}
