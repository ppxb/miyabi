package service

import (
	"container/list"
	"context"
	"sync"
	"time"
)

// responseCache holds immutable JavDB models, never projected library/task state.
// Pending loads are shared; the last departing caller cancels the upstream request.
type responseCache[T any] struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	entries  map[string]*list.Element
	recent   list.List
	pending  map[string]*cacheLoad[T]
}

type cacheEntry[T any] struct {
	key     string
	value   T
	expires time.Time
}

type cacheLoad[T any] struct {
	ctx     context.Context
	cancel  context.CancelFunc
	waiters int
	done    chan struct{}
	value   T
	err     error
}

func newResponseCache[T any](capacity int, ttl time.Duration) *responseCache[T] {
	return &responseCache[T]{
		capacity: capacity, ttl: ttl,
		entries: make(map[string]*list.Element),
		pending: make(map[string]*cacheLoad[T]),
	}
}

func (cache *responseCache[T]) get(ctx context.Context, key string, load func(context.Context) (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	cache.mu.Lock()
	if element, ok := cache.entries[key]; ok {
		entry := element.Value.(cacheEntry[T])
		if time.Now().Before(entry.expires) {
			cache.recent.MoveToFront(element)
			cache.mu.Unlock()
			return entry.value, nil
		}
		cache.remove(element)
	}
	call, exists := cache.pending[key]
	if !exists {
		shared, cancel := context.WithCancel(context.WithoutCancel(ctx))
		call = &cacheLoad[T]{ctx: shared, cancel: cancel, done: make(chan struct{})}
		cache.pending[key] = call
		go cache.load(key, call, load)
	}
	call.waiters++
	cache.mu.Unlock()
	defer cache.release(key, call)

	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-call.done:
		return call.value, call.err
	}
}

func (cache *responseCache[T]) load(key string, call *cacheLoad[T], load func(context.Context) (T, error)) {
	value, err := load(call.ctx)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	call.value, call.err = value, err
	if cache.pending[key] == call {
		delete(cache.pending, key)
		if err == nil {
			if len(cache.entries) == cache.capacity {
				cache.remove(cache.recent.Back())
			}
			cache.entries[key] = cache.recent.PushFront(cacheEntry[T]{
				key: key, value: value, expires: time.Now().Add(cache.ttl),
			})
		}
	}
	close(call.done)
	call.cancel()
}

func (cache *responseCache[T]) release(key string, call *cacheLoad[T]) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	call.waiters--
	if call.waiters == 0 && cache.pending[key] == call {
		delete(cache.pending, key)
		call.cancel()
	}
}

func (cache *responseCache[T]) remove(element *list.Element) {
	delete(cache.entries, element.Value.(cacheEntry[T]).key)
	cache.recent.Remove(element)
}

func cachedJavDB[T any](ctx context.Context, service *DiscoverService, cache *responseCache[T], key string, load func(context.Context) (T, error)) (T, error) {
	return cache.get(ctx, key, func(ctx context.Context) (T, error) {
		value, err := load(ctx)
		if err == nil {
			err = service.persistActiveRoute(ctx)
		}
		return value, err
	})
}
