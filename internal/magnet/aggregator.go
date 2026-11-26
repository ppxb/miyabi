package magnet

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

const (
	defaultAggregatorTimeout = 8 * time.Second
	// breakerThreshold consecutive failures put a source into a cooldown of
	// breakerCooldown, so an unreachable source stops costing a full timeout on
	// every query. The first query after the cooldown probes it again.
	breakerThreshold = 3
	breakerCooldown  = 5 * time.Minute
)

// errCoolingDown reports a source left out while its breaker is open.
var errCoolingDown = errors.New("cooling down after repeated failures")

type breaker struct {
	failures  int
	openUntil time.Time
}

// Aggregator queries every source concurrently, merges records that share an
// infohash, enriches them with inferred tags, and orders the result.
type Aggregator struct {
	sources  []Source
	timeout  time.Duration
	mu       sync.Mutex
	breakers []breaker
}

// NewAggregator creates an Aggregator. Each source gets its own timeout;
// a non-positive timeout uses the default.
func NewAggregator(sources []Source, timeout time.Duration) *Aggregator {
	if timeout <= 0 {
		timeout = defaultAggregatorTimeout
	}
	return &Aggregator{sources: sources, timeout: timeout, breakers: make([]breaker, len(sources))}
}

func (a *Aggregator) admit(i int) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !time.Now().Before(a.breakers[i].openUntil)
}

// record updates a source's breaker with a query outcome and reports whether
// this failure opened it.
func (a *Aggregator) record(i int, err error) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	b := &a.breakers[i]
	if err == nil {
		b.failures = 0
		return false
	}
	b.failures++
	if b.failures < breakerThreshold {
		return false
	}
	b.openUntil = time.Now().Add(breakerCooldown)
	return true
}

type queryResult struct {
	source  string
	magnets []domain.Magnet
	err     error
}

// ErrSkipped indicates a source was skipped and did not participate in querying.
var ErrSkipped = errors.New("magnet source skipped")

// Find returns the merged magnets of all sources. One failing source is only
// logged; the call fails only when every active source failed.
func (a *Aggregator) Find(ctx context.Context, ref domain.MovieRef) ([]domain.Magnet, error) {
	magnets, _, err := a.FindDetailed(ctx, ref)
	return magnets, err
}

// FindDetailed returns the merged magnets, whether any active source failed (partial),
// and any fatal error.
func (a *Aggregator) FindDetailed(ctx context.Context, ref domain.MovieRef) ([]domain.Magnet, bool, error) {
	if len(a.sources) == 0 {
		return nil, false, nil
	}

	results := make([]queryResult, len(a.sources))
	var wg sync.WaitGroup
	for i, src := range a.sources {
		wg.Go(func() {
			if !a.admit(i) {
				results[i] = queryResult{source: src.Name(), err: errCoolingDown}
				return
			}
			sourceCtx, cancel := context.WithTimeout(ctx, a.timeout)
			defer cancel()
			magnets, err := src.Find(sourceCtx, ref)
			results[i] = queryResult{source: src.Name(), magnets: magnets, err: err}
			// A switched-off or challenged source says nothing about its health,
			// and a caller that went away is not the source's fault.
			if ctx.Err() != nil || errors.Is(err, ErrSkipped) || errors.Is(err, domain.ErrBlocked) {
				return
			}
			if a.record(i, err) {
				slog.WarnContext(ctx, "magnet source cooling down after repeated failures",
					"source", src.Name(), "failures", breakerThreshold, "cooldown", breakerCooldown)
			}
		})
	}
	wg.Wait()

	var failures []error
	var skipped int
	var blocked int
	merged := make(map[string]*domain.Magnet)
	var order []string
	for _, res := range results {
		if errors.Is(res.err, ErrSkipped) {
			skipped++
			continue
		}
		// A source answering with a bot challenge has no usable content; treat it
		// as an empty result instead of a failure so the movie shows "no magnets".
		if errors.Is(res.err, domain.ErrBlocked) {
			slog.WarnContext(ctx, "magnet source blocked by challenge; treating as no result",
				"source", res.source, "code", ref.Code)
			blocked++
			continue
		}
		if errors.Is(res.err, errCoolingDown) {
			failures = append(failures, fmt.Errorf("%s: %w", res.source, res.err))
			continue
		}
		if res.err != nil {
			slog.WarnContext(ctx, "magnet source query failed", "source", res.source, "code", ref.Code, "error", res.err)
			failures = append(failures, fmt.Errorf("%s: %w", res.source, res.err))
			continue
		}
		for _, m := range res.magnets {
			if existing, ok := merged[m.Hash]; ok {
				mergeMagnet(existing, m, res.source)
				continue
			}
			entry := m
			merged[m.Hash] = &entry
			order = append(order, m.Hash)
		}
	}

	activeSources := len(a.sources) - skipped - blocked
	if activeSources > 0 && len(failures) == activeSources {
		return nil, false, domain.E(domain.KindUpstream, "所有磁力来源均不可用", errors.Join(failures...))
	}

	// Zero magnets despite some failed sources is reported as an empty result
	// with partial=true: the failures are logged per source above, and callers
	// treat "no magnets yet" as a normal answer rather than an error.
	result := make([]domain.Magnet, 0, len(merged))
	for _, hash := range order {
		item := merged[hash]
		ApplyInference(item)
		result = append(result, *item)
	}

	// 字幕 > 高清 > 体积 > 文件数; JavDB-listed first on ties, then newest.
	slices.SortStableFunc(result, func(a, b domain.Magnet) int {
		if sa, sb := HasSubtitle(a), HasSubtitle(b); sa != sb {
			if sa {
				return -1
			}
			return 1
		}
		if ha, hb := IsHD(a), IsHD(b); ha != hb {
			if ha {
				return -1
			}
			return 1
		}
		return compareMagnets(a, b)
	})
	return result, len(failures) > 0 || blocked > 0, nil
}

// mergeMagnet folds a second record of the same infohash into existing:
// sources accumulate, flags OR together, size and file count take the maximum,
// the first non-empty name wins, and the earliest date is kept.
func mergeMagnet(existing *domain.Magnet, incoming domain.Magnet, source string) {
	existing.Sources = mergeSources(existing.Sources, append([]string{source}, incoming.Sources...))
	existing.HasSubtitle = existing.HasSubtitle || incoming.HasSubtitle
	existing.HD = existing.HD || incoming.HD
	existing.Size = max(existing.Size, incoming.Size)
	existing.FilesCount = max(existing.FilesCount, incoming.FilesCount)

	if existing.Name == "" {
		existing.Name = incoming.Name
	}

	if existing.CreatedAt == "" || (incoming.CreatedAt != "" && incoming.CreatedAt < existing.CreatedAt) {
		existing.CreatedAt = incoming.CreatedAt
	}
	for _, tag := range incoming.Tags {
		if !slices.Contains(existing.Tags, tag) {
			existing.Tags = append(existing.Tags, tag)
		}
	}
}

func mergeSources(existing, incoming []string) []string {
	for _, source := range incoming {
		if source == "" || slices.Contains(existing, source) {
			continue
		}
		existing = append(existing, source)
	}
	return existing
}
