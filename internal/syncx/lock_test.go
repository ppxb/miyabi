package syncx

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
)

func TestContextLockRejectsUnlockWithoutLock(t *testing.T) {
	for _, initialized := range []bool{false, true} {
		name := "zero value"
		if initialized {
			name = "double unlock"
		}
		t.Run(name, func(t *testing.T) {
			var lock ContextLock
			if initialized {
				if err := lock.Lock(t.Context()); err != nil {
					t.Fatal(err)
				}
				lock.Unlock()
			}
			defer func() {
				if got := recover(); got != "syncx: unlock of unlocked ContextLock" {
					t.Fatalf("unexpected unlock panic: %v", got)
				}
				if !lock.TryLock() {
					t.Fatal("misuse panic left the lock unusable")
				}
				lock.Unlock()
			}()
			lock.Unlock()
		})
	}
}

func TestContextLockCanceledWaitPreservesOwnerAndAllowsReuse(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var lock ContextLock
		if !lock.TryLock() || lock.TryLock() {
			t.Fatal("TryLock did not distinguish free and held locks")
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		result := make(chan error, 1)
		go func() { result <- lock.Lock(ctx) }()
		synctest.Wait()
		select {
		case err := <-result:
			t.Fatalf("waiter returned before cancellation: %v", err)
		default:
		}
		cancel()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("waiter did not report cancellation: %v", err)
		}
		if lock.TryLock() {
			t.Fatal("canceled waiter released the owner's lock")
		}
		lock.Unlock()
		if err := lock.Lock(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("already canceled context acquired a free lock: %v", err)
		}
		if err := lock.Lock(t.Context()); err != nil {
			t.Fatalf("canceled attempt left the lock held: %v", err)
		}
		lock.Unlock()
	})
}

func TestContextLockSerializesWaiters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var lock ContextLock
		if err := lock.Lock(t.Context()); err != nil {
			t.Fatal(err)
		}
		var group sync.WaitGroup
		var entered atomic.Int32
		var value int
		for range 8 {
			group.Go(func() {
				if err := lock.Lock(t.Context()); err != nil {
					t.Error(err)
					return
				}
				defer lock.Unlock()
				entered.Add(1)
				value++
			})
		}
		synctest.Wait()
		if entered.Load() != 0 {
			t.Fatal("waiters entered before the owner unlocked")
		}
		lock.Unlock()
		group.Wait()
		if value != 8 || entered.Load() != 8 {
			t.Fatalf("lost serialized updates: value=%d entered=%d", value, entered.Load())
		}
	})
}
