// Package syncx holds small synchronisation primitives shared across packages.
package syncx

import (
	"context"
	"sync"
)

// ContextLock is a mutex whose waiters give up when their context ends.
// The zero value is ready to use.
type ContextLock struct {
	once sync.Once
	gate chan struct{}
}

func (lock *ContextLock) init() {
	lock.once.Do(func() { lock.gate = make(chan struct{}, 1) })
}

// Lock blocks until the lock is acquired or ctx is done. A lock acquired at
// the same moment ctx ends is released again so the caller never holds it.
func (lock *ContextLock) Lock(ctx context.Context) error {
	lock.init()
	select {
	case lock.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			lock.Unlock()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TryLock acquires the lock only if it is free.
func (lock *ContextLock) TryLock() bool {
	lock.init()
	select {
	case lock.gate <- struct{}{}:
		return true
	default:
		return false
	}
}

// Unlock releases the lock. It panics if the lock is not held.
func (lock *ContextLock) Unlock() {
	lock.init()
	select {
	case <-lock.gate:
	default:
		panic("syncx: unlock of unlocked ContextLock")
	}
}
