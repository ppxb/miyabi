package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ppxb/miyabi/internal/pan"
)

func (service *PanService) refreshTokens(ctx context.Context, expected panSnapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key := fmt.Sprintf("%d:%d", expected.credentialVersion, expected.tokenVersion)
	result := service.refresh.DoChan(key, func() (any, error) {
		if !service.startWork() {
			return nil, context.Canceled
		}
		defer service.work.Done()
		current, err := service.credentials(expected)
		if err != nil || current.tokenVersion != expected.tokenVersion {
			return nil, err
		}
		// Refresh tokens rotate too. Finish saving the new pair even when all
		// waiting HTTP requests have been canceled.
		tokenContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
		defer cancel()
		tokens, err := service.client.RefreshToken(tokenContext, current.tokens.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("refresh 115 credentials: %w", err)
		}
		if err := service.commit.Lock(tokenContext); err != nil {
			return nil, err
		}
		defer service.commit.Unlock()
		current, err = service.credentials(expected)
		if err != nil || current.tokenVersion != expected.tokenVersion {
			return nil, err
		}
		if err := saveSetting(tokenContext, service.database, panCredentialsSetting, tokens); err != nil {
			return nil, err
		}
		service.mu.Lock()
		service.tokens = tokens
		service.tokenVersion++
		service.mu.Unlock()
		return nil, nil
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case completed := <-result:
		return completed.Err
	}
}

// The account generation is pinned for the entire operation. Only explicit
// authorization rejection is replayed; ambiguous mutation failures are not.
func withPanToken[T any](ctx context.Context, service *PanService, expected panSnapshot, request func(string) (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	current, err := service.credentials(expected)
	if err != nil {
		return zero, err
	}
	if current.closed {
		return zero, context.Canceled
	}
	refreshed := time.Until(current.tokens.ExpiresAt) <= 30*time.Second
	if refreshed {
		if err := service.refreshTokens(ctx, current); err != nil {
			return zero, err
		}
		current, err = service.credentials(expected)
		if err != nil {
			return zero, err
		}
	}
	if current.closed {
		return zero, context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	value, err := request(current.tokens.AccessToken)
	if errors.Is(err, pan.ErrUnauthorized) && !refreshed {
		if err := service.refreshTokens(ctx, current); err != nil {
			return zero, err
		}
		current, err = service.credentials(expected)
		if err != nil {
			return zero, err
		}
		if current.closed {
			return zero, context.Canceled
		}
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		return request(current.tokens.AccessToken)
	}
	return value, err
}

// Check again after waiting for a token refresh, immediately before each
// source-bound request. In particular, a queued write must not start on a
// mount that was replaced while its credentials were being refreshed.
func withPanSourceToken[T any](ctx context.Context, service *PanService, expected panSnapshot, request func(string) (T, error)) (T, error) {
	return withPanToken(ctx, service, expected, func(token string) (T, error) {
		if _, err := service.sourceState(expected.source(), expected.authorizationVersion); err != nil {
			var zero T
			return zero, err
		}
		return request(token)
	})
}
