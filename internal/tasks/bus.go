package tasks

import (
	"fmt"
	"sync"
	"time"
)

// Revisions are monotonic change counters broadcast to SSE clients so the
// frontend can decide which queries to refetch.
type TaskRevisions struct {
	Library uint64 `json:"library"`
	Offline uint64 `json:"offline"`
	Monitor uint64 `json:"monitor"`
}

// Change is a bitmask naming which revision counters an event bumps.
type Change uint8

const (
	ChangeLibrary Change = 1 << iota
	ChangeOffline
	ChangeMonitor
)

type workerSubscription struct {
	updates chan struct{}
	kinds   map[Kind]struct{}
}

// Bus separates queued-work wakeups from task and business updates for SSE.
type Bus struct {
	mu          sync.Mutex
	instance    string
	version     uint64
	revisions   TaskRevisions
	subscribers map[chan struct{}]struct{}
	workers     map[*workerSubscription]struct{}
}

func NewBus() *Bus {
	return &Bus{
		instance:    fmt.Sprintf("%d", time.Now().UnixNano()),
		subscribers: make(map[chan struct{}]struct{}),
		workers:     make(map[*workerSubscription]struct{}),
	}
}

// Instance identifies this process. Revisions reset to zero on restart, so SSE
// clients use a changed instance to re-baseline instead of missing updates.
func (b *Bus) Instance() string { return b.instance }

// Subscribe registers an SSE listener. Notifications are coalesced.
func (b *Bus) Subscribe() (<-chan struct{}, func()) {
	return b.subscribe(b.subscribers)
}

// SubscribePool registers one worker for queued-work wakeups. With no kinds
// the worker observes every kind; otherwise only matching wakeups reach it.
func (b *Bus) SubscribePool(kinds ...Kind) (<-chan struct{}, func()) {
	sub := &workerSubscription{
		updates: make(chan struct{}, 1),
		kinds:   make(map[Kind]struct{}, len(kinds)),
	}
	for _, kind := range kinds {
		sub.kinds[kind] = struct{}{}
	}
	b.mu.Lock()
	b.workers[sub] = struct{}{}
	b.mu.Unlock()
	return sub.updates, func() {
		b.mu.Lock()
		delete(b.workers, sub)
		b.mu.Unlock()
	}
}

func (b *Bus) subscribe(listeners map[chan struct{}]struct{}) (<-chan struct{}, func()) {
	updates := make(chan struct{}, 1)
	b.mu.Lock()
	listeners[updates] = struct{}{}
	b.mu.Unlock()
	return updates, func() {
		b.mu.Lock()
		delete(listeners, updates)
		b.mu.Unlock()
	}
}

// WakePool announces committed queued work without invalidating UI snapshots.
// With no kinds every worker wakes; otherwise only workers subscribed to a
// matching kind do.
func (b *Bus) WakePool(kinds ...Kind) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for worker := range b.workers {
		if !worker.matches(kinds) {
			continue
		}
		select {
		case worker.updates <- struct{}{}:
		default:
		}
	}
}

func (s *workerSubscription) matches(kinds []Kind) bool {
	if len(s.kinds) == 0 || len(kinds) == 0 {
		return true
	}
	for _, kind := range kinds {
		if _, ok := s.kinds[kind]; ok {
			return true
		}
	}
	return false
}

// NotifyUI refreshes task views without bumping business revisions or waking workers.
func (b *Bus) NotifyUI() { b.publish(0) }

func (b *Bus) NotifyLibraryChanged() { b.publish(ChangeLibrary) }
func (b *Bus) NotifyOfflineChanged() { b.publish(ChangeOffline) }
func (b *Bus) NotifyMonitorChanged() { b.publish(ChangeMonitor) }

func (b *Bus) Revisions() TaskRevisions {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.revisions
}

// Version changes on every UI notification, including task progress updates.
func (b *Bus) Version() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.version
}

func (b *Bus) publish(change Change) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.version++
	if change&ChangeLibrary != 0 {
		b.revisions.Library++
	}
	if change&ChangeOffline != 0 {
		b.revisions.Offline++
	}
	if change&ChangeMonitor != 0 {
		b.revisions.Monitor++
	}
	signal(b.subscribers)
}

func signal(listeners map[chan struct{}]struct{}) {
	for subscriber := range listeners {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}
