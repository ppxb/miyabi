package tasks

import "sync"

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

// Bus fans task notifications out to the worker pools and SSE subscribers.
type Bus struct {
	mu          sync.Mutex
	version     uint64
	revisions   TaskRevisions
	subscribers map[chan struct{}]struct{}
}

func NewBus() *Bus {
	return &Bus{subscribers: make(map[chan struct{}]struct{})}
}

// Subscribe registers a worker pool or SSE listener. Notifications are coalesced.
func (b *Bus) Subscribe() (<-chan struct{}, func()) {
	updates := make(chan struct{}, 1)
	b.mu.Lock()
	b.subscribers[updates] = struct{}{}
	b.mu.Unlock()
	return updates, func() {
		b.mu.Lock()
		delete(b.subscribers, updates)
		b.mu.Unlock()
	}
}

// Notify wakes all subscribers without bumping business revisions.
func (b *Bus) Notify() { b.publish(0) }

func (b *Bus) NotifyLibraryChanged() { b.publish(ChangeLibrary) }
func (b *Bus) NotifyOfflineChanged() { b.publish(ChangeOffline) }
func (b *Bus) NotifyMonitorChanged() { b.publish(ChangeMonitor) }

func (b *Bus) Revisions() TaskRevisions {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.revisions
}

// Version changes on every notification, including task progress updates.
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
	for subscriber := range b.subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}
