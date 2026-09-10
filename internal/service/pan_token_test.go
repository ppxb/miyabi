package service

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/pan"
)

func TestPanConcurrentUnauthorizedRequestsShareRefresh(t *testing.T) {
	library, client := panConcurrencyFixture(t)
	drive, refreshed := library.drive, panTestTokens("refreshed")
	state := drive.snapshot()
	const requests = 12
	entered, refreshStarted := make(chan struct{}, requests), make(chan struct{}, requests)
	reject, rejectNow := panTestGate(t)
	lateReject, rejectLate := panTestGate(t)
	holdRefresh, releaseRefresh := panTestGate(t)
	var refreshes atomic.Int32
	client.refreshToken = func(context.Context, string) (pan.Tokens, error) {
		refreshes.Add(1)
		refreshStarted <- struct{}{}
		<-holdRefresh
		return refreshed, nil
	}
	finished := make(chan error, requests)
	for i := range requests {
		go func() {
			_, err := withPanToken(t.Context(), drive, state, func(token string) (string, error) {
				if token == state.tokens.AccessToken {
					entered <- struct{}{}
					if i == requests-1 {
						<-lateReject
					} else {
						<-reject
					}
					return "", pan.ErrUnauthorized
				}
				if token != refreshed.AccessToken {
					return "", fmt.Errorf("retried with unexpected credentials")
				}
				return "ok", nil
			})
			finished <- err
		}()
	}
	for range requests {
		awaitPan(t, entered)
	}
	rejectNow()
	awaitPan(t, refreshStarted)
	releaseRefresh()
	for range requests - 1 {
		if err := awaitPan(t, finished); err != nil {
			t.Fatal(err)
		}
	}
	// This 401 belongs to the old token but arrives after refresh has finished.
	rejectLate()
	if err := awaitPan(t, finished); err != nil {
		t.Fatal(err)
	}
	if refreshes.Load() != 1 {
		t.Fatalf("refresh calls = %d, want 1", refreshes.Load())
	}
	assertPanTokens(t, drive, refreshed)
}

func TestPanCanceledRefreshWaiterStillPersistsRotatedTokens(t *testing.T) {
	library, client := panConcurrencyFixture(t)
	drive, refreshed := library.drive, panTestTokens("refreshed")
	drive.tokens.ExpiresAt = time.Now()
	started := make(chan context.Context, 1)
	hold, release := panTestGate(t)
	client.refreshToken = func(ctx context.Context, _ string) (pan.Tokens, error) {
		started <- ctx
		<-hold
		return refreshed, ctx.Err()
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var requests atomic.Int32
	finished := make(chan error, 1)
	go func() {
		_, err := withPanToken(ctx, drive, drive.snapshot(), func(string) (struct{}, error) {
			requests.Add(1)
			return struct{}{}, nil
		})
		finished <- err
	}()
	upstream := awaitPan(t, started)
	cancel()
	if err := awaitPan(t, finished); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter = %v", err)
	}
	if deadline, ok := upstream.Deadline(); !ok || time.Until(deadline) > 45*time.Second || upstream.Err() != nil {
		t.Fatal("shared refresh must survive its waiter with a bounded deadline")
	}
	release()
	saved := make(chan struct{})
	go func() { drive.work.Wait(); close(saved) }()
	awaitPan(t, saved)
	assertPanTokens(t, drive, refreshed)
	if requests.Load() != 0 {
		t.Fatal("a canceled waiter started its original request")
	}
}

func TestPanMountChangeDuringRefreshStopsQueuedSourceRequest(t *testing.T) {
	library, client := panConcurrencyFixture(t)
	drive, refreshed := library.drive, panTestTokens("refreshed")
	drive.tokens.ExpiresAt = time.Now()
	started := make(chan struct{}, 1)
	hold, release := panTestGate(t)
	client.refreshToken = func(context.Context, string) (pan.Tokens, error) {
		started <- struct{}{}
		<-hold
		return refreshed, nil
	}
	var requests atomic.Int32
	finished := make(chan error, 1)
	go func() {
		_, err := withPanSourceToken(t.Context(), drive, drive.snapshot(), func(string) (struct{}, error) {
			requests.Add(1)
			return struct{}{}, nil
		})
		finished <- err
	}()
	awaitPan(t, started)
	if err := drive.ClearDirectory(t.Context()); err != nil {
		t.Fatal(err)
	}
	release()
	if err := awaitPan(t, finished); !errors.Is(err, errPanSourceChanged) {
		t.Fatalf("request on replaced source = %v", err)
	}
	assertPanTokens(t, drive, refreshed)
	if requests.Load() != 0 {
		t.Fatal("queued source request ran after the mount changed")
	}
}

func TestPanRefreshCannotRestoreReplacedCredentials(t *testing.T) {
	for _, action := range []string{"disconnect", "login"} {
		t.Run(action, func(t *testing.T) {
			library, client := panConcurrencyFixture(t)
			drive := library.drive
			drive.tokens.ExpiresAt = time.Now()
			started := make(chan struct{}, 1)
			hold, release := panTestGate(t)
			client.refreshToken = func(context.Context, string) (pan.Tokens, error) {
				started <- struct{}{}
				<-hold
				return panTestTokens("stale-refresh"), nil
			}
			finished := make(chan error, 1)
			go func() {
				_, err := drive.Files(t.Context(), "10", 1)
				finished <- err
			}()
			awaitPan(t, started)
			var want pan.Tokens
			if action == "disconnect" {
				if _, err := drive.Disconnect(t.Context()); err != nil {
					t.Fatal(err)
				}
			} else {
				want = panTestTokens("new-login")
				client.exchangeToken = func(context.Context, *pan.Login) (pan.Tokens, error) { return want, nil }
				login, err := drive.BeginLogin(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if status, err := drive.LoginStatus(t.Context(), login.ID); err != nil || status.State != pan.LoginAuthorized {
					t.Fatalf("new login = %+v, %v", status, err)
				}
			}
			release()
			if err := awaitPan(t, finished); !errors.Is(err, pan.ErrUnauthorized) {
				t.Fatalf("old credential operation = %v", err)
			}
			assertPanTokens(t, drive, want)
		})
	}
}

func TestPanOnlyRetriesExplicitAuthorizationRejection(t *testing.T) {
	for _, failure := range []error{errors.New("connection reset after sending request"), context.DeadlineExceeded, pan.ErrUnauthorized} {
		t.Run(failure.Error(), func(t *testing.T) {
			library, client := panConcurrencyFixture(t)
			requests, refreshes := 0, 0
			client.refreshToken = func(context.Context, string) (pan.Tokens, error) {
				refreshes++
				return panTestTokens("refreshed"), nil
			}
			_, err := withPanToken(t.Context(), library.drive, library.drive.snapshot(), func(string) (struct{}, error) {
				requests++
				return struct{}{}, failure
			})
			wantRequests, wantRefreshes := 1, 0
			if errors.Is(failure, pan.ErrUnauthorized) {
				wantRequests, wantRefreshes = 2, 1
			}
			if !errors.Is(err, failure) || requests != wantRequests || refreshes != wantRefreshes {
				t.Fatalf("request retries=%d refreshes=%d err=%v", requests, refreshes, err)
			}
		})
	}
}

func TestPanCloseFinishesRefreshBeforeStoppingQueuedRequests(t *testing.T) {
	library, client := panConcurrencyFixture(t)
	drive, tokens := library.drive, panTestTokens("refreshed")
	drive.tokens.ExpiresAt = time.Now()
	started := make(chan struct{}, 1)
	hold, release := panTestGate(t)
	client.refreshToken = func(context.Context, string) (pan.Tokens, error) {
		started <- struct{}{}
		<-hold
		return tokens, nil
	}
	var requests atomic.Int32
	client.list = func(context.Context, string, string, int, int) (pan.FilePage, error) {
		requests.Add(1)
		return pan.FilePage{}, nil
	}
	finished := make(chan error, 1)
	go func() {
		_, err := drive.Files(t.Context(), "10", 1)
		finished <- err
	}()
	awaitPan(t, started)
	closed := make(chan struct{})
	go func() { drive.Close(); close(closed) }()
	awaitPanCondition(t, func() bool { return drive.snapshot().closed })
	select {
	case <-closed:
		t.Fatal("Close returned before the rotated credentials were saved")
	default:
	}
	release()
	awaitPan(t, closed)
	if err := awaitPan(t, finished); !errors.Is(err, context.Canceled) {
		t.Fatalf("request queued before Close = %v", err)
	}
	assertPanTokens(t, drive, tokens)
	if requests.Load() != 0 {
		t.Fatal("request started after the service closed")
	}
}
