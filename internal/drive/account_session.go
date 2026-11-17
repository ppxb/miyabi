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

// accountSession is a non-primary 115 account session backed by the account
// registry, with its own token cache and refresh.
type accountSession struct {
	drive     *Drive
	accountID string
	source    domain.LibrarySource
}

func (s *accountSession) Source() domain.LibrarySource { return s.source }

func (s *accountSession) AuthorizationVersion() uint64 { return 0 }

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

func (s *accountSession) List(ctx context.Context, dirID string, offset int) (pan.FilePage, error) {
	return accountWithToken(ctx, s, func(token string) (pan.FilePage, error) {
		return s.drive.client.List(ctx, token, dirID, offset, 100)
	})
}

func (s *accountSession) Info(ctx context.Context, fileID string) (pan.FileInfo, error) {
	return accountWithToken(ctx, s, func(token string) (pan.FileInfo, error) {
		return s.drive.client.Info(ctx, token, fileID)
	})
}

func (s *accountSession) Read(ctx context.Context, pickCode string, limit int64) ([]byte, error) {
	return accountWithToken(ctx, s, func(token string) ([]byte, error) {
		return s.drive.client.ReadMetadata(ctx, token, pickCode, limit)
	})
}

func (s *accountSession) PlayURL(ctx context.Context, pickCode, userAgent string) ([]pan.PlaySource, error) {
	return accountWithToken(ctx, s, func(token string) ([]pan.PlaySource, error) {
		return s.drive.client.PlayURL(ctx, token, pickCode, userAgent)
	})
}

func (s *accountSession) DownloadURL(ctx context.Context, pickCode, userAgent string) (string, error) {
	return accountWithToken(ctx, s, func(token string) (string, error) {
		return s.drive.client.DownloadURL(ctx, token, pickCode, userAgent)
	})
}

func (s *accountSession) AddOffline(ctx context.Context, magnet string) (string, error) {
	return accountWithToken(ctx, s, func(token string) (string, error) {
		return s.drive.client.AddOffline(ctx, token, magnet, s.source.Directory.ID)
	})
}

func (s *accountSession) RemoveOffline(ctx context.Context, hash string) error {
	_, err := accountWithToken(ctx, s, func(token string) (struct{}, error) {
		return struct{}{}, s.drive.client.RemoveOffline(ctx, token, hash)
	})
	return err
}

func (s *accountSession) OfflineTasks(ctx context.Context, page int) (pan.OfflinePage, error) {
	return accountWithToken(ctx, s, func(token string) (pan.OfflinePage, error) {
		return s.drive.client.OfflineTasks(ctx, token, page)
	})
}

func (s *accountSession) Commit(ctx context.Context, fn func(tx *ent.Tx) error) error {
	if err := s.drive.commit.Lock(ctx); err != nil {
		return err
	}
	defer s.drive.commit.Unlock()
	return ent.WithTx(ctx, s.drive.database, fn)
}

func (s *accountSession) WithSource(ctx context.Context, fn func() error) error {
	if err := s.drive.commit.Lock(ctx); err != nil {
		return err
	}
	defer s.drive.commit.Unlock()
	return fn()
}

func (s *accountSession) CommitAccount(ctx context.Context, fn func(tx *ent.Tx) error) error {
	return s.Commit(ctx, fn)
}
