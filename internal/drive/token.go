package drive

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/pan"
)

func (d *Drive) refreshTokens(ctx context.Context, expected snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key := fmt.Sprintf("%d:%d", expected.credentialVersion, expected.tokenVersion)
	result := d.refresh.DoChan(key, func() (any, error) {
		done, ok := d.StartWork()
		if !ok {
			return nil, context.Canceled
		}
		defer done()
		current, err := d.credentials(expected)
		if err != nil || current.tokenVersion != expected.tokenVersion {
			return nil, err
		}
		tokenContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), upstreamTimeout)
		defer cancel()
		tokens, err := d.client.RefreshToken(tokenContext, current.tokens.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("refresh 115 credentials: %w", err)
		}
		if err := d.commit.Lock(tokenContext); err != nil {
			return nil, err
		}
		defer d.commit.Unlock()
		current, err = d.credentials(expected)
		if err != nil || current.tokenVersion != expected.tokenVersion {
			return nil, err
		}
		if err := database.SaveSetting(tokenContext, d.database, credentialsSetting, tokens); err != nil {
			return nil, err
		}
		d.mu.Lock()
		d.tokens = tokens
		d.tokenVersion++
		d.mu.Unlock()
		return nil, nil
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case completed := <-result:
		return completed.Err
	}
}

func withPanToken[T any](ctx context.Context, d *Drive, expected snapshot, request func(snapshot) (T, error)) (T, error) {
	var zero T
	refreshed := false
	for {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		current, err := d.credentials(expected)
		if err != nil {
			return zero, err
		}
		if current.closed {
			return zero, context.Canceled
		}
		expiresSoon := !current.tokens.ExpiresAt.IsZero() && time.Until(current.tokens.ExpiresAt) <= 30*time.Second
		if refreshed || !expiresSoon {
			value, err := request(current)
			if refreshed || !errors.Is(err, pan.ErrUnauthorized) {
				return value, err
			}
		}
		if err := d.refreshTokens(ctx, current); err != nil {
			return zero, err
		}
		// The next iteration rechecks cancellation and credentials, then returns
		// the request result even if the refreshed token is rejected.
		refreshed = true
	}
}

func withPanSourceToken[T any](ctx context.Context, d *Drive, expected snapshot, request func(string) (T, error)) (T, error) {
	return withPanToken(ctx, d, expected, func(current snapshot) (T, error) {
		// Check the same snapshot that supplied this attempt's credentials.
		if !current.matchesSource(expected.source(), expected.authorizationVersion) {
			var zero T
			return zero, ErrSourceChanged
		}
		return request(current.tokens.AccessToken)
	})
}
