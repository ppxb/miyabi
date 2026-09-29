package app

import (
	"log/slog"

	"github.com/ppxb/miyabi/internal/tasks"
)

func newTaskPools(service *tasks.Service, logger *slog.Logger) []*tasks.Pool {
	return []*tasks.Pool{
		// Scans and metadata writes stay ordered on one library worker.
		tasks.NewPool(service.Queue(), service, service.Registry(),
			[]tasks.Kind{tasks.KindScan, tasks.KindScrape, tasks.KindCover}, 1, logger),
		// Batches retain serial submission and pacing without occupying the library worker.
		tasks.NewPool(service.Queue(), service, service.Registry(),
			[]tasks.Kind{tasks.KindSubscriptionBatch}, 1, logger),
	}
}
