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
	client      Client
	provider    string
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
func (s *accountSession) checkCredentials(ctx context.Context) error {
	if s.drive.accountCredVersion(s.accountID) != s.credVersion {
		return ErrSourceChanged
	}
	tokens, err := s.drive.accountToken(ctx, s.accountID)
	if err != nil {
		return err
	}
	if tokens.AccessToken == "" {
		return pan.ErrUnauthorized
	}
	return nil
}

func accountWithToken[T any](ctx context.Context, s *accountSession, request func(string) (T, error)) (T, error) {
	tokens, err := s.drive.accountToken(ctx, s.accountID)
	if err != nil {
		var zero T
		return zero, err
	}
	return useToken(ctx, s, tokens, request)
}

func useToken[T any](ctx context.Context, s *accountSession, tokens pan.Tokens, request func(string) (T, error)) (T, error) {
	var zero T
	refreshed := false
	for {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
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
		next, err := s.drive.accountToken(ctx, s.accountID)
		if err != nil {
			return zero, err
		}
		tokens = next
		refreshed = true
	}
}

func (d *Drive) refreshAccountTokens(ctx context.Context, accountID string) error {
	result := d.accountRefresh.DoChan(accountID, func() (any, error) {
		refreshContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), upstreamTimeout)
		defer cancel()
		record, err := d.accountRecord(refreshContext, accountID)
		if err != nil {
			return nil, err
		}
		tokens := record.Tokens
		if tokens.RefreshToken == "" {
			return nil, pan.ErrUnauthorized
		}
		client, ok := d.providerClient(record.Provider)
		if !ok {
			return nil, domain.E(domain.KindInvalid, "存储账号的来源不受支持", nil)
		}
		next, err := client.RefreshToken(refreshContext, tokens.RefreshToken)
		if err != nil {
			if errors.Is(err, pan.ErrUnauthorized) {
				_ = d.removeAccount(refreshContext, accountID)
			}
			return nil, fmt.Errorf("refresh pan credentials: %w", err)
		}
		if err := d.updateAccountTokens(refreshContext, accountID, next); err != nil {
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
// token refresh independent of the authorization version. The initial source
// check and token read share one account row; the check after the request still
// detects a mount replaced while it was in flight.
func readAccount[T any](ctx context.Context, s *accountSession, request func(string) (T, error)) (T, error) {
	var zero T
	if s.drive.accountAuthVersion(s.accountID) != s.authVersion {
		return zero, ErrSourceChanged
	}
	record, err := s.drive.accountRecord(ctx, s.accountID)
	if err != nil {
		return zero, err
	}
	// Mount reports ok=false for a blank directory, so reject that here too.
	if record.Directory.ID == "" || record.Directory.ID != s.source.Directory.ID {
		return zero, ErrSourceChanged
	}
	value, err := useToken(ctx, s, record.Tokens, request)
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
		return s.client.List(ctx, token, dirID, offset, 100)
	})
}

func (s *accountSession) Info(ctx context.Context, fileID string) (pan.FileInfo, error) {
	return readAccount(ctx, s, func(token string) (pan.FileInfo, error) {
		return s.client.Info(ctx, token, fileID)
	})
}

func (s *accountSession) Read(ctx context.Context, pickCode string, limit int64) ([]byte, error) {
	return readAccount(ctx, s, func(token string) ([]byte, error) {
		return s.client.ReadMetadata(ctx, token, pickCode, limit)
	})
}

func (s *accountSession) PlayURL(ctx context.Context, pickCode, userAgent string) ([]pan.PlaySource, error) {
	return readAccount(ctx, s, func(token string) ([]pan.PlaySource, error) {
		return s.client.PlayURL(ctx, token, pickCode, userAgent)
	})
}

func (s *accountSession) DownloadURL(ctx context.Context, pickCode, userAgent string) (string, error) {
	return readAccount(ctx, s, func(token string) (string, error) {
		return s.client.DownloadURL(ctx, token, pickCode, userAgent)
	})
}

func (s *accountSession) AddOffline(ctx context.Context, magnet string) (string, error) {
	if err := s.checkSource(ctx); err != nil {
		return "", err
	}
	return accountWithToken(ctx, s, func(token string) (string, error) {
		return s.client.AddOffline(ctx, token, magnet, s.source.Directory.ID)
	})
}

func (s *accountSession) RemoveOffline(ctx context.Context, hash string) error {
	if err := s.checkSource(ctx); err != nil {
		return err
	}
	_, err := accountWithToken(ctx, s, func(token string) (struct{}, error) {
		return struct{}{}, s.client.RemoveOffline(ctx, token, hash)
	})
	if err != nil {
		return err
	}
	return s.checkSource(ctx)
}

func (s *accountSession) OfflineTasks(ctx context.Context, page int) (pan.OfflinePage, error) {
	if err := s.checkCredentials(ctx); err != nil {
		return pan.OfflinePage{}, err
	}
	result, err := accountWithToken(ctx, s, func(token string) (pan.OfflinePage, error) {
		return s.client.OfflineTasks(ctx, token, page)
	})
	if err != nil {
		return result, err
	}
	if s.provider == ProviderGuangya {
		// Guangya's task list does not expose the target directory. All tasks
		// observed through this session belong to the mounted source.
		for i := range result.Tasks {
			result.Tasks[i].DirectoryID = s.source.Directory.ID
		}
	}
	return result, nil
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
	if err := s.checkCredentials(ctx); err != nil {
		return err
	}
	if s.drive.closedState() {
		return context.Canceled
	}
	return ent.WithTx(ctx, s.drive.database, fn)
}
