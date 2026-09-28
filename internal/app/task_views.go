package app

import (
	"context"
	"slices"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/library"
	"github.com/ppxb/miyabi/internal/monitor"
	"github.com/ppxb/miyabi/internal/tasks"
)

// taskViews composes business-owned projections with the task notification bus.
type taskViews struct {
	*tasks.Service
	library *library.Service
	monitor *monitor.Service
}

func (v *taskViews) List(ctx context.Context) ([]domain.TaskInfo, error) {
	scans, err := v.library.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	batches, err := v.monitor.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	result := append(scans, batches...)
	slices.SortFunc(result, func(a, b domain.TaskInfo) int {
		aActive := a.Status == "queued" || a.Status == "running"
		bActive := b.Status == "queued" || b.Status == "running"
		if aActive != bActive {
			if aActive {
				return -1
			}
			return 1
		}
		if a.Status == "running" && b.Status == "queued" {
			return -1
		}
		if a.Status == "queued" && b.Status == "running" {
			return 1
		}
		return b.ID - a.ID
	})
	return result, nil
}
