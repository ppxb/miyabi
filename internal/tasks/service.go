package tasks

import (
	"context"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
)

// Service is what business packages hold: enqueue entry points, workflow
// projections for the API, and the notification surface. Execution wiring
// (Queue, Registry, Pool) is composed by the application root.
type Service struct {
	database *ent.Client
	queue    *Queue
	bus      *Bus
	registry *Registry
}

func NewService(database *ent.Client, registry *Registry) *Service {
	bus := NewBus()
	return &Service{database: database, queue: NewQueue(database, registry, bus), bus: bus, registry: registry}
}

func (s *Service) Queue() *Queue       { return s.queue }
func (s *Service) Bus() *Bus           { return s.bus }
func (s *Service) Registry() *Registry { return s.registry }

func (s *Service) List(ctx context.Context) ([]TaskInfo, error) {
	return ListWorkflows(ctx, s.database)
}

func (s *Service) Info(ctx context.Context, id int) (TaskInfo, error) {
	return WorkflowInfo(ctx, s.database, id)
}

func (s *Service) Workflows(ctx context.Context, records []*ent.Task) ([]TaskInfo, error) {
	return WorkflowInfos(ctx, s.database, records)
}

// EnqueueScan reuses a queued or running full scan of the source or creates
// one. It serves manual retries, where any active scan is good enough.
func (s *Service) EnqueueScan(ctx context.Context, source domain.LibrarySource) (TaskInfo, error) {
	return s.enqueueScan(ctx, source, task.StatusQueued, task.StatusRunning)
}

// EnqueueFreshScan queues a full scan that observes the current mount. Only a
// scan that has not started yet is reused: a running one may have been issued
// against an earlier mount of the same directory and is about to fail its
// source check.
func (s *Service) EnqueueFreshScan(ctx context.Context, source domain.LibrarySource) (TaskInfo, error) {
	return s.enqueueScan(ctx, source, task.StatusQueued)
}

func (s *Service) enqueueScan(ctx context.Context, source domain.LibrarySource, reusable ...task.Status) (TaskInfo, error) {
	if err := s.queue.Lock(ctx); err != nil {
		return TaskInfo{}, err
	}
	defer s.queue.Unlock()
	record, err := EnsureScanTask(ctx, s.database.Task, source, reusable...)
	if err != nil {
		return TaskInfo{}, err
	}
	s.bus.Notify()
	return ScanTaskInfo(record)
}

func (s *Service) Subscribe() (<-chan struct{}, func()) { return s.bus.Subscribe() }
func (s *Service) Revisions() TaskRevisions             { return s.bus.Revisions() }
func (s *Service) Notify()                              { s.bus.Notify() }
func (s *Service) NotifyLibraryChanged()                { s.bus.NotifyLibraryChanged() }
func (s *Service) NotifyOfflineChanged()                { s.bus.NotifyOfflineChanged() }
func (s *Service) NotifyMonitorChanged()                { s.bus.NotifyMonitorChanged() }
