package app

import (
	"log/slog"

	"github.com/ppxb/miyabi/internal/tasks"
)

func newTaskPools(service *tasks.Service, libraryWorkers int, logger *slog.Logger) []*tasks.Pool {
	return []*tasks.Pool{
		tasks.NewPool(service.Queue(), service.Bus(), service.Registry(),
			[]tasks.Kind{tasks.KindScan, tasks.KindScrape, tasks.KindCover}, libraryWorkers, logger),
		// Batches retain serial submission and pacing without occupying the library worker.
		tasks.NewPool(service.Queue(), service.Bus(), service.Registry(),
			[]tasks.Kind{tasks.KindSubscriptionBatch}, 1, logger),
	}
}
