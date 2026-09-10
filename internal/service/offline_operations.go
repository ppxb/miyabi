package service

import (
	"context"
	"sync"
)

type offlineOperation struct {
	lock  contextLock
	users int
}

// Unrelated magnets can progress independently. References include waiters,
// so cancellation and release cannot replace a lock that is still in use.
type offlineOperations struct {
	mu      sync.Mutex
	entries map[string]*offlineOperation
}

func (operations *offlineOperations) Lock(ctx context.Context, accountID, hash string) (func(), error) {
	key := accountID + ":" + hash
	operations.mu.Lock()
	if operations.entries == nil {
		operations.entries = make(map[string]*offlineOperation)
	}
	entry := operations.entries[key]
	if entry == nil {
		entry = &offlineOperation{}
		operations.entries[key] = entry
	}
	entry.users++
	operations.mu.Unlock()
	release := func() {
		operations.mu.Lock()
		defer operations.mu.Unlock()
		entry.users--
		if entry.users == 0 {
			delete(operations.entries, key)
		}
	}
	if err := entry.lock.Lock(ctx); err != nil {
		release()
		return nil, err
	}
	return func() { entry.lock.Unlock(); release() }, nil
}
