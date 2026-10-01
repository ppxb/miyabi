package app

import (
	"log/slog"

	"github.com/ppxb/miyabi/internal/tasks"
)

func newTaskPools(service *tasks.Service, logger *slog.Logger) []*tasks.Pool {
	return []*tasks.Pool{
		// Directory traversal cannot be starved by a large scraping backlog.
		tasks.NewPool(service.Queue(), service, service.Registry(),
			[]tasks.Kind{tasks.KindScan}, 1, logger),
		// Per-source rate limits and resource keys bound parallel scraping.
		tasks.NewPool(service.Queue(), service, service.Registry(),
			[]tasks.Kind{tasks.KindScrape}, 2, logger),
		// Batches retain serial submission and pacing without occupying the library worker.
		tasks.NewPool(service.Queue(), service, service.Registry(),
			[]tasks.Kind{tasks.KindSubscriptionBatch}, 1, logger),
	}
}
