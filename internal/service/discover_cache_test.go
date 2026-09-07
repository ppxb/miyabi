package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestResponseCacheSharesLoadsWithoutSharingCallerCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := newResponseCache[int](2, time.Minute)
		var calls atomic.Int32
		finish := make(chan struct{})
		load := func(ctx context.Context) (int, error) {
			calls.Add(1)
			select {
			case <-finish:
				return 42, nil
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}
		first, cancel := context.WithCancel(t.Context())
		firstResult := make(chan error, 1)
		go func() {
			_, err := cache.get(first, "same", load)
			firstResult <- err
		}()
		synctest.Wait()
		secondResult := make(chan int, 1)
		go func() {
			value, err := cache.get(t.Context(), "same", load)
			if err != nil {
				t.Error(err)
			}
			secondResult <- value
		}()
		synctest.Wait()
		cancel()
		if err := <-firstResult; !errors.Is(err, context.Canceled) {
			t.Fatalf("first caller error = %v", err)
		}
		close(finish)
		if got := <-secondResult; got != 42 || calls.Load() != 1 {
			t.Fatalf("second caller = %d, upstream calls = %d", got, calls.Load())
		}
	})
}

func TestResponseCacheCancelsAbandonedLoad(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := newResponseCache[int](2, time.Minute)
		ctx, cancel := context.WithCancel(t.Context())
		upstreamCanceled := make(chan struct{})
		finishOldLoad := make(chan struct{})
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			cache.get(ctx, "same", func(ctx context.Context) (int, error) {
				<-ctx.Done()
				close(upstreamCanceled)
				<-finishOldLoad
				return 1, nil
			})
		}()
		synctest.Wait()
		cancel()
		<-upstreamCanceled
		<-finished
		value, err := cache.get(t.Context(), "same", func(context.Context) (int, error) { return 7, nil })
		if value != 7 || err != nil {
			t.Fatalf("next load = %d, error = %v", value, err)
		}
		close(finishOldLoad)
		synctest.Wait()
		value, err = cache.get(t.Context(), "same", func(context.Context) (int, error) {
			t.Error("replacement result was not cached")
			return 0, nil
		})
		if value != 7 || err != nil {
			t.Fatalf("abandoned result overwrote cache: %d, %v", value, err)
		}
	})
}

func TestResponseCacheExpiresAndEvictsLeastRecentlyUsed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := newResponseCache[int](2, time.Minute)
		loads := map[string]int{}
		get := func(key string) int {
			t.Helper()
			value, err := cache.get(t.Context(), key, func(context.Context) (int, error) {
				loads[key]++
				return loads[key], nil
			})
			if err != nil {
				t.Fatal(err)
			}
			return value
		}
		get("a")
		get("b")
		get("a")
		get("c")
		if get("a") != 1 || get("b") != 2 {
			t.Fatalf("unexpected eviction: loads = %v", loads)
		}
		time.Sleep(time.Minute)
		if get("b") != 3 {
			t.Fatalf("expired result was reused: loads = %v", loads)
		}
	})
}

func TestResponseCacheDoesNotCacheErrors(t *testing.T) {
	cache := newResponseCache[int](2, time.Minute)
	upstream := errors.New("upstream unavailable")
	_, err := cache.get(t.Context(), "same", func(context.Context) (int, error) { return 0, upstream })
	if !errors.Is(err, upstream) {
		t.Fatalf("error = %v", err)
	}
	value, err := cache.get(t.Context(), "same", func(context.Context) (int, error) { return 9, nil })
	if value != 9 || err != nil {
		t.Fatalf("retry = %d, error = %v", value, err)
	}
}
