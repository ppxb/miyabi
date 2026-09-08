package worker

import (
	"context"
	"log/slog"
	"time"
)

type OfflineSyncer interface {
	Sync(context.Context) error
}

func RunOffline(ctx context.Context, offline OfflineSyncer, logger *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if err := offline.Sync(ctx); err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "sync 115 offline tasks", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
