package magnet

import (
	"context"
	"sync/atomic"

	"github.com/ppxb/miyabi/internal/domain"
)

// Toggle wraps a Source with an atomically switchable enabled flag. A disabled
// source reports ErrSkipped so the aggregator excludes it without an error.
type Toggle struct {
	source  Source
	enabled atomic.Bool
}

// NewToggle wraps source with the given initial enabled state.
func NewToggle(source Source, enabled bool) *Toggle {
	t := &Toggle{source: source}
	t.enabled.Store(enabled)
	return t
}

// Name identifies the wrapped source.
func (t *Toggle) Name() string { return t.source.Name() }

// Enabled reports the current switch state.
func (t *Toggle) Enabled() bool { return t.enabled.Load() }

// SetEnabled flips the switch without rebuilding the aggregator.
func (t *Toggle) SetEnabled(enabled bool) { t.enabled.Store(enabled) }

// Find delegates to the wrapped source unless it is switched off.
func (t *Toggle) Find(ctx context.Context, ref domain.MovieRef) ([]domain.Magnet, error) {
	if !t.enabled.Load() {
		return nil, ErrSkipped
	}
	return t.source.Find(ctx, ref)
}
