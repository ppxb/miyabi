package drive

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/pan"
)

func TestConcurrentUnauthorizedRequestsShareRefresh(t *testing.T) {
	d, client := mountedTestDrive(t)
	refreshed := testTokens("refreshed")
	state := d.snapshot()
	const requests = 12
	entered, refreshStarted := make(chan struct{}, requests), make(chan struct{}, requests)
	reject, rejectNow := testGate(t)
	lateReject, rejectLate := testGate(t)
	holdRefresh, releaseRefresh := testGate(t)
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
			_, err := withPanToken(t.Context(), d, state, func(token string) (string, error) {
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
		await(t, entered)
	}
	rejectNow()
	await(t, refreshStarted)
	releaseRefresh()
	for range requests - 1 {
		if err := await(t, finished); err != nil {
			t.Fatal(err)
		}
	}
	// This 401 belongs to the old token but arrives after refresh has finished.
	rejectLate()
	if err := await(t, finished); err != nil {
		t.Fatal(err)
	}
	if refreshes.Load() != 1 {
		t.Fatalf("refresh calls = %d, want 1", refreshes.Load())
	}
	assertTokens(t, d, refreshed)
}

func TestCanceledRefreshWaiterStillPersistsRotatedTokens(t *testing.T) {
	d, client := mountedTestDrive(t)
	refreshed := testTokens("refreshed")
	expireTokens(d)
	started := make(chan context.Context, 1)
	hold, release := testGate(t)
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
		_, err := withPanToken(ctx, d, d.snapshot(), func(string) (struct{}, error) {
			requests.Add(1)
			return struct{}{}, nil
		})
		finished <- err
	}()
	upstream := await(t, started)
	cancel()
	if err := await(t, finished); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter = %v", err)
	}
	if deadline, ok := upstream.Deadline(); !ok || time.Until(deadline) > 45*time.Second || upstream.Err() != nil {
		t.Fatal("shared refresh must survive its waiter with a bounded deadline")
	}
	release()
	saved := make(chan struct{})
	go func() { d.work.Wait(); close(saved) }()
	await(t, saved)
	assertTokens(t, d, refreshed)
	if requests.Load() != 0 {
		t.Fatal("a canceled waiter started its original request")
	}
}

func TestMountChangeDuringRefreshStopsQueuedSourceRequest(t *testing.T) {
	d, client := mountedTestDrive(t)
	refreshed := testTokens("refreshed")
	expireTokens(d)
	started := make(chan struct{}, 1)
	hold, release := testGate(t)
	client.refreshToken = func(context.Context, string) (pan.Tokens, error) {
		started <- struct{}{}
		<-hold
		return refreshed, nil
	}
	var requests atomic.Int32
	finished := make(chan error, 1)
	go func() {
		_, err := withPanSourceToken(t.Context(), d, d.snapshot(), func(string) (struct{}, error) {
			requests.Add(1)
			return struct{}{}, nil
		})
		finished <- err
	}()
	await(t, started)
	if err := d.ClearDirectory(t.Context()); err != nil {
		t.Fatal(err)
	}
	release()
	if err := await(t, finished); !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("request on replaced source = %v", err)
	}
	assertTokens(t, d, refreshed)
	if requests.Load() != 0 {
		t.Fatal("queued source request ran after the mount changed")
	}
}

func TestRefreshCannotRestoreReplacedCredentials(t *testing.T) {
	for _, action := range []string{"disconnect", "login"} {
		t.Run(action, func(t *testing.T) {
			d, client := mountedTestDrive(t)
			expireTokens(d)
			started := make(chan struct{}, 1)
			hold, release := testGate(t)
			client.refreshToken = func(context.Context, string) (pan.Tokens, error) {
				started <- struct{}{}
				<-hold
				return testTokens("stale-refresh"), nil
			}
			finished := make(chan error, 1)
			go func() {
				_, err := d.Files(t.Context(), "10", 1)
				finished <- err
			}()
			await(t, started)
			var want pan.Tokens
			if action == "disconnect" {
				if _, err := d.Disconnect(t.Context()); err != nil {
					t.Fatal(err)
				}
			} else {
				want = testTokens("new-login")
				client.exchangeToken = func(context.Context, *pan.Login) (pan.Tokens, error) { return want, nil }
				loginTestAccount(t, d)
			}
			release()
			if err := await(t, finished); !errors.Is(err, pan.ErrUnauthorized) {
				t.Fatalf("old credential operation = %v", err)
			}
			assertTokens(t, d, want)
		})
	}
}

func TestOnlyRetriesExplicitAuthorizationRejection(t *testing.T) {
	for _, failure := range []error{errors.New("connection reset after sending request"), context.DeadlineExceeded, pan.ErrUnauthorized} {
		t.Run(failure.Error(), func(t *testing.T) {
			d, client := mountedTestDrive(t)
			requests, refreshes := 0, 0
			client.refreshToken = func(context.Context, string) (pan.Tokens, error) {
				refreshes++
				return testTokens("refreshed"), nil
			}
			_, err := withPanToken(t.Context(), d, d.snapshot(), func(string) (struct{}, error) {
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

func TestProactiveRefreshDoesNotRetryRejectedRequest(t *testing.T) {
	d, client := mountedTestDrive(t)
	expireTokens(d)
	refreshed := testTokens("refreshed")
	requests, refreshes := 0, 0
	client.refreshToken = func(context.Context, string) (pan.Tokens, error) {
		refreshes++
		return refreshed, nil
	}
	_, err := withPanToken(t.Context(), d, d.snapshot(), func(token string) (struct{}, error) {
		requests++
		if token != refreshed.AccessToken {
			t.Errorf("request used token before proactive refresh")
		}
		return struct{}{}, pan.ErrUnauthorized
	})
	if !errors.Is(err, pan.ErrUnauthorized) || requests != 1 || refreshes != 1 {
		t.Fatalf("requests=%d refreshes=%d err=%v", requests, refreshes, err)
	}
}

func TestCanceledAuthorizationRejectionDoesNotStartRefresh(t *testing.T) {
	d, client := mountedTestDrive(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var refreshes atomic.Int32
	client.refreshToken = func(context.Context, string) (pan.Tokens, error) {
		refreshes.Add(1)
		return testTokens("unexpected"), nil
	}
	requests := 0
	_, err := withPanToken(ctx, d, d.snapshot(), func(string) (struct{}, error) {
		requests++
		cancel()
		return struct{}{}, pan.ErrUnauthorized
	})
	d.Close()
	if !errors.Is(err, context.Canceled) || requests != 1 || refreshes.Load() != 0 {
		t.Fatalf("requests=%d refreshes=%d err=%v", requests, refreshes.Load(), err)
	}
}

func TestCloseFinishesRefreshBeforeStoppingQueuedRequests(t *testing.T) {
	d, client := mountedTestDrive(t)
	tokens := testTokens("refreshed")
	expireTokens(d)
	started := make(chan struct{}, 1)
	hold, release := testGate(t)
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
		_, err := d.Files(t.Context(), "10", 1)
		finished <- err
	}()
	await(t, started)
	closed := make(chan struct{})
	go func() { d.Close(); close(closed) }()
	awaitCondition(t, func() bool { return d.snapshot().closed })
	select {
	case <-closed:
		t.Fatal("Close returned before the rotated credentials were saved")
	default:
	}
	release()
	await(t, closed)
	if err := await(t, finished); !errors.Is(err, context.Canceled) {
		t.Fatalf("request queued before Close = %v", err)
	}
	assertTokens(t, d, tokens)
	if requests.Load() != 0 {
		t.Fatal("request started after the service closed")
	}
}
