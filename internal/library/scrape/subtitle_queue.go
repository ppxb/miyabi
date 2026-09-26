package scrape

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/subtitle"
)

// SubtitleTask exports the subtitles of one movie after its artwork and .strm are written.
type SubtitleTask struct {
	MovieID     int
	MetaPayload MetadataPayload
	Target      subtitle.Target
}

// SubtitleQueue is a bounded, concurrency-controlled background worker queue for subtitle processing.
type SubtitleQueue struct {
	tasks   chan SubtitleTask
	pending map[int]bool
	mu      sync.Mutex
	service *Service
	logger  *slog.Logger
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

const (
	defaultSubtitleQueueCapacity = 256
	defaultSubtitleConcurrency   = 2
)

// newSubtitleQueue initializes a bounded queue and starts dedicated worker goroutines.
func newSubtitleQueue(service *Service, concurrency, capacity int, logger *slog.Logger) *SubtitleQueue {
	if concurrency <= 0 {
		concurrency = defaultSubtitleConcurrency
	}
	if capacity <= 0 {
		capacity = defaultSubtitleQueueCapacity
	}
	if logger == nil {
		logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(context.Background())
	q := &SubtitleQueue{
		tasks:   make(chan SubtitleTask, capacity),
		pending: make(map[int]bool),
		service: service,
		logger:  logger,
		ctx:     ctx,
		cancel:  cancel,
	}

	for range concurrency {
		q.wg.Add(1)
		go q.worker()
	}

	return q
}

// Enqueue adds a subtitle task to the queue if not already pending and queue has capacity.
// It returns true if enqueued, false if deduplicated or dropped due to queue overflow.
func (q *SubtitleQueue) Enqueue(task SubtitleTask) bool {
	q.mu.Lock()
	if q.ctx != nil && q.ctx.Err() != nil {
		q.mu.Unlock()
		return false
	}
	if q.pending[task.MovieID] {
		q.mu.Unlock()
		return false // Deduplicate: already queued or running
	}
	q.pending[task.MovieID] = true
	q.mu.Unlock()

	select {
	case q.tasks <- task:
		return true
	default:
		// Queue full: safely drop and clear pending state to prevent memory leak
		q.mu.Lock()
		delete(q.pending, task.MovieID)
		q.mu.Unlock()
		if q.logger != nil {
			q.logger.WarnContext(q.ctx, "subtitle task queue is full; dropping task",
				"code", task.Target.Code,
				"movie_id", task.MovieID,
			)
		}
		return false
	}
}

// Close gracefully stops the worker pool and waits for active tasks to finish.
func (q *SubtitleQueue) Close() {
	q.cancel()
	q.wg.Wait()
}

func (q *SubtitleQueue) worker() {
	defer q.wg.Done()
	for {
		select {
		case <-q.ctx.Done():
			return
		case task, ok := <-q.tasks:
			if !ok {
				return
			}
			q.process(task)
			q.mu.Lock()
			delete(q.pending, task.MovieID)
			q.mu.Unlock()
		}
	}
}

func (q *SubtitleQueue) process(task SubtitleTask) {
	if q.service.subtitles == nil {
		return
	}
	// Online providers are slow; one movie must not hold a worker indefinitely.
	ctx, cancel := context.WithTimeout(q.ctx, 2*time.Minute)
	defer cancel()

	written, err := q.service.subtitles.Export(ctx, nil, task.MovieID, task.Target)
	if err != nil {
		q.logger.WarnContext(ctx, "export subtitles", "code", task.Target.Code, "error", err)
	}
	if written > 0 && q.service.mediaNotifier != nil {
		q.service.mediaNotifier.NotifyUpdated(task.Target.Dir)
	}
}
