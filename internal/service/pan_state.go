package service

import (
	"context"
	"errors"
	"sync"

	"github.com/ppxb/miyabi/internal/pan"
)

var errPanSourceChanged = errors.New("媒体目录或登录账号已变更，请重新扫描")

// contextLock has a usable zero value and lets waiting callers leave promptly.
type contextLock struct {
	once sync.Once
	gate chan struct{}
}

func (lock *contextLock) Lock(ctx context.Context) error {
	lock.once.Do(func() { lock.gate = make(chan struct{}, 1) })
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

func (lock *contextLock) Unlock() { <-lock.gate }

func (lock *contextLock) TryLock() bool {
	lock.once.Do(func() { lock.gate = make(chan struct{}, 1) })
	select {
	case lock.gate <- struct{}{}:
		return true
	default:
		return false
	}
}

type panSnapshot struct {
	tokens               pan.Tokens
	directory            panLibraryDirectory
	authorizationVersion uint64
	credentialVersion    uint64
	tokenVersion         uint64
	closed               bool
}

func (service *PanService) snapshot() panSnapshot {
	service.mu.Lock()
	defer service.mu.Unlock()
	return panSnapshot{
		tokens: service.tokens, directory: service.directory,
		authorizationVersion: service.authorizationVersion, credentialVersion: service.credentialVersion,
		tokenVersion: service.tokenVersion, closed: service.closed,
	}
}

func (state panSnapshot) source() LibrarySource {
	return LibrarySource{AccountID: state.directory.AccountID, Directory: state.directory.PanLibraryDirectory}
}

func (state panSnapshot) matchesSource(source LibrarySource, version uint64) bool {
	return state.authorizationVersion == version && state.directory.AccountID == source.AccountID && state.directory.ID == source.Directory.ID
}

func (service *PanService) credentials(expected panSnapshot) (panSnapshot, error) {
	current := service.snapshot()
	if current.credentialVersion != expected.credentialVersion || current.tokens.AccessToken == "" {
		return panSnapshot{}, pan.ErrUnauthorized
	}
	return current, nil
}

func (service *PanService) sourceState(source LibrarySource, version uint64) (panSnapshot, error) {
	state := service.snapshot()
	if !state.matchesSource(source, version) {
		return panSnapshot{}, errPanSourceChanged
	}
	return state, nil
}

func (service *PanService) verifiedSource(ctx context.Context) (panSnapshot, error) {
	state := service.snapshot()
	account, err := service.account(ctx, state)
	if err != nil {
		return panSnapshot{}, err
	}
	if _, err := service.sourceState(state.source(), state.authorizationVersion); err != nil {
		return panSnapshot{}, err
	}
	if state.directory.ID == "" || state.directory.AccountID != account.ID {
		return panSnapshot{}, ErrMediaDirectoryRequired
	}
	return state, nil
}

// State changes and source-bound database commits take this gate before any
// transaction. Playback only reads mu and never waits for database writes.
func (service *PanService) commitSource(ctx context.Context, source LibrarySource, version uint64, write func() error) error {
	if err := service.commit.Lock(ctx); err != nil {
		return err
	}
	defer service.commit.Unlock()
	if _, err := service.sourceState(source, version); err != nil {
		return err
	}
	return write()
}
