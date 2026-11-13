//go:build !windows

package tray

import "context"

// Run falls back to a plain foreground server on platforms without a tray.
func Run(ctx context.Context, cancel context.CancelFunc, _ Options, serve ServeFunc) error {
	defer cancel()
	return serve(ctx)
}
