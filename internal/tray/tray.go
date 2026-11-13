package tray

import "context"

// Options describes the tray icon and the local web UI it exposes.
type Options struct {
	Title   string
	URL     string
	DataDir string
}

// ServeFunc runs the application until ctx is cancelled.
type ServeFunc func(ctx context.Context) error
