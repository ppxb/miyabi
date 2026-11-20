package drive

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/pan"
)

// accountSession is a 115 account session backed by the account registry, with
// its own token cache/refresh and credential/authorization version guards.
type accountSession struct {
	drive       *Drive
	accountID   string
	source      domain.LibrarySource
	credVersion uint64
	authVersion uint64
}

func (s *accountSession) Source() domain.LibrarySource { return s.source }

func (s *accountSession) AuthorizationVersion() uint64 { return s.authVersion }

// checkSource rejects reads tied to an older login or mount.
func (s *accountSession) checkSource(ctx context.Context) error {
	if s.drive.accountAuthVersion(s.accountID) != s.authVersion {
		return ErrSourceChanged
	}
	source, ok, err := s.drive.Mount(ctx, s.accountID)
	if err != nil {
		return err
	}
	if !ok || source.Directory.ID != s.source.Directory.ID {
		return ErrSourceChanged
	}
	return nil
}

// checkCredentials rejects commits authorized by older credentials.
func (s *accountSession) checkCredentials() error {
	if s.drive.accountCredVersion(s.accountID) != s.credVersion {
		return ErrSourceChanged
	}
	if s.drive.accountToken(s.accountID).AccessToken == "" {
		return pan.ErrUnauthorized
	}
	return nil
}

func accountWithToken[T any](ctx context.Context, s *accountSession, request func(string) (T, error)) (T, error) {
	var zero T
	refreshed := false
	for {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		tokens := s.drive.accountToken(s.accountID)
		if tokens.AccessToken == "" {
			return zero, pan.ErrUnauthorized
		}
		expiresSoon := !tokens.ExpiresAt.IsZero() && time.Until(tokens.ExpiresAt) <= 30*time.Second
		if refreshed || !expiresSoon {
			value, err := request(tokens.AccessToken)
			if refreshed || !errors.Is(err, pan.ErrUnauthorized) {
				return value, err
			}
		}
		if err := s.drive.refreshAccountTokens(ctx, s.accountID); err != nil {
			return zero, err
		}
		refreshed = true
	}
}

func (d *Drive) refreshAccountTokens(ctx context.Context, accountID string) error {
	result := d.accountRefresh.DoChan(accountID, func() (any, error) {
		tokens := d.accountToken(accountID)
		if tokens.RefreshToken == "" {
			return nil, pan.ErrUnauthorized
		}
		refreshContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), upstreamTimeout)
		defer cancel()
		next, err := d.client.RefreshToken(refreshContext, tokens.RefreshToken)
		if err != nil {
			if errors.Is(err, pan.ErrUnauthorized) {
				_ = d.removeAccount(refreshContext, accountID)
			}
			return nil, fmt.Errorf("refresh 115 credentials: %w", err)
		}
		if err := d.saveAccountTokens(refreshContext, accountID, next); err != nil {
			return nil, err
		}
		return nil, nil
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case completed := <-result:
		return completed.Err
	}
}

// readAccount discards read results from a replaced account/mount while keeping
// token refresh independent of the authorization version.
func readAccount[T any](ctx context.Context, s *accountSession, request func(string) (T, error)) (T, error) {
	var zero T
	if err := s.checkSource(ctx); err != nil {
		return zero, err
	}
	value, err := accountWithToken(ctx, s, request)
	if err != nil {
		return zero, err
	}
	if err := s.checkSource(ctx); err != nil {
		return zero, err
	}
	return value, nil
}

func (s *accountSession) List(ctx context.Context, dirID string, offset int) (pan.FilePage, error) {
	return readAccount(ctx, s, func(token string) (pan.FilePage, error) {
		return s.drive.client.List(ctx, token, dirID, offset, 100)
	})
}

func (s *accountSession) Info(ctx context.Context, fileID string) (pan.FileInfo, error) {
	return readAccount(ctx, s, func(token string) (pan.FileInfo, error) {
		return s.drive.client.Info(ctx, token, fileID)
	})
}

func (s *accountSession) Read(ctx context.Context, pickCode string, limit int64) ([]byte, error) {
	return readAccount(ctx, s, func(token string) ([]byte, error) {
		return s.drive.client.ReadMetadata(ctx, token, pickCode, limit)
	})
}

func (s *accountSession) PlayURL(ctx context.Context, pickCode, userAgent string) ([]pan.PlaySource, error) {
	return readAccount(ctx, s, func(token string) ([]pan.PlaySource, error) {
		return s.drive.client.PlayURL(ctx, token, pickCode, userAgent)
	})
}

func (s *accountSession) DownloadURL(ctx context.Context, pickCode, userAgent string) (string, error) {
	return readAccount(ctx, s, func(token string) (string, error) {
		return s.drive.client.DownloadURL(ctx, token, pickCode, userAgent)
	})
}

func (s *accountSession) AddOffline(ctx context.Context, magnet string) (string, error) {
	if err := s.checkSource(ctx); err != nil {
		return "", err
	}
	return accountWithToken(ctx, s, func(token string) (string, error) {
		return s.drive.client.AddOffline(ctx, token, magnet, s.source.Directory.ID)
	})
}

func (s *accountSession) RemoveOffline(ctx context.Context, hash string) error {
	if err := s.checkSource(ctx); err != nil {
		return err
	}
	_, err := accountWithToken(ctx, s, func(token string) (struct{}, error) {
		return struct{}{}, s.drive.client.RemoveOffline(ctx, token, hash)
	})
	if err != nil {
		return err
	}
	return s.checkSource(ctx)
}

func (s *accountSession) OfflineTasks(ctx context.Context, page int) (pan.OfflinePage, error) {
	if err := s.checkCredentials(); err != nil {
		return pan.OfflinePage{}, err
	}
	return accountWithToken(ctx, s, func(token string) (pan.OfflinePage, error) {
		return s.drive.client.OfflineTasks(ctx, token, page)
	})
}

func (s *accountSession) Commit(ctx context.Context, fn func(tx *ent.Tx) error) error {
	return s.WithSource(ctx, func() error { return ent.WithTx(ctx, s.drive.database, fn) })
}

func (s *accountSession) WithSource(ctx context.Context, fn func() error) error {
	if err := s.drive.commit.Lock(ctx); err != nil {
		return err
	}
	defer s.drive.commit.Unlock()
	if err := s.checkSource(ctx); err != nil {
		return err
	}
	return fn()
}

func (s *accountSession) CommitAccount(ctx context.Context, fn func(tx *ent.Tx) error) error {
	if err := s.drive.commit.Lock(ctx); err != nil {
		return err
	}
	defer s.drive.commit.Unlock()
	if err := s.checkCredentials(); err != nil {
		return err
	}
	if s.drive.closedState() {
		return context.Canceled
	}
	return ent.WithTx(ctx, s.drive.database, fn)
}
