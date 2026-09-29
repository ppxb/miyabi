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
// The callback runs under the commit lock, but not the state lock. It must not
// open sessions or commit through Drive. A rejected mount restores its old state.
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
