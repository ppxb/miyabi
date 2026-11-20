package drive

import (
	"context"

	"github.com/ppxb/miyabi/internal/domain"
)

// MountEvent signals that a media directory has been mounted or cleared.
type MountEvent struct {
	Source domain.LibrarySource
}

// MountListener receives mount notifications.
type MountListener func(ctx context.Context, event MountEvent) error

// SetMountListener replaces the synchronous mount callback; nil clears it.
// The callback runs without the state lock and is not wrapped in a commit lock;
// it must not open sessions or commit through Drive. An error is returned to the
// caller, but any mount change made before the call is not rolled back.
func (d *Drive) SetMountListener(listener MountListener) {
	d.mu.Lock()
	d.mountListener = listener
	d.mu.Unlock()
}

func (d *Drive) publishMount(ctx context.Context, source domain.LibrarySource) error {
	d.mu.Lock()
	listener := d.mountListener
	d.mu.Unlock()
	if listener == nil {
		return nil
	}
	return listener(ctx, MountEvent{Source: source})
}
