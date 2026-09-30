package tasks

import "github.com/ppxb/miyabi/internal/ent"

// Service exposes task execution wiring and notifications to business packages.
type Service struct {
	queue    *Queue
	bus      *Bus
	registry *Registry
}

func NewService(database *ent.Client, registry *Registry) *Service {
	bus := NewBus()
	return &Service{queue: &Queue{database: database, registry: registry, bus: bus}, bus: bus, registry: registry}
}

func (s *Service) Queue() *Queue       { return s.queue }
func (s *Service) Registry() *Registry { return s.registry }

func (s *Service) Subscribe() (<-chan struct{}, func()) { return s.bus.Subscribe() }
func (s *Service) Revisions() TaskRevisions             { return s.bus.Revisions() }
func (s *Service) Version() uint64                      { return s.bus.Version() }
func (s *Service) Notify()                              { s.bus.Notify() }
func (s *Service) NotifyLibraryChanged()                { s.bus.NotifyLibraryChanged() }
func (s *Service) NotifyOfflineChanged()                { s.bus.NotifyOfflineChanged() }
func (s *Service) NotifyMonitorChanged()                { s.bus.NotifyMonitorChanged() }
