package drive

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ppxb/miyabi/internal/pan"
)

func TestConcurrentAccountLookupsShareResult(t *testing.T) {
	for _, failure := range []error{nil, context.DeadlineExceeded} {
		name := "success"
		if failure != nil {
			name = "failure"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				hold := make(chan struct{})
				var calls atomic.Int32
				account := pan.Account{ID: "100"}
				client := &stubClient{account: func(context.Context, string) (pan.Account, error) {
					calls.Add(1)
					<-hold
					return account, failure
				}}
				d := &Drive{client: client, tokens: testTokens("current")}
				defer d.Close()
				state := d.snapshot()
				const waiters = 12
				finished := make(chan error, waiters)
				for range waiters {
					go func() {
						got, err := d.verifyAccount(t.Context(), state)
						if err == nil && got.ID != account.ID {
							t.Errorf("account=%+v", got)
						}
						finished <- err
					}()
				}
				synctest.Wait()
				close(hold)
				for range waiters {
					if err := <-finished; !errors.Is(err, failure) {
						t.Fatalf("lookup error=%v, want %v", err, failure)
					}
				}
				if calls.Load() != 1 {
					t.Fatalf("concurrent lookups sent %d requests", calls.Load())
				}
				if _, err := d.verifyAccount(t.Context(), state); !errors.Is(err, failure) {
					t.Fatal(err)
				}
				want := int32(1)
				if failure != nil {
					want = 2 // Transient failures must not become cached account status.
				}
				if calls.Load() != want {
					t.Fatalf("subsequent lookup calls=%d, want %d", calls.Load(), want)
				}
				if failure == nil {
					time.Sleep(accountCacheTTL)
					if _, err := d.verifyAccount(t.Context(), state); err != nil || calls.Load() != 2 {
						t.Fatalf("expired cache calls=%d err=%v", calls.Load(), err)
					}
				}
			})
		})
	}
}

func TestCanceledAccountWaiterDoesNotCancelSharedLookup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hold := make(chan struct{})
		var upstream context.Context
		var calls atomic.Int32
		client := &stubClient{account: func(ctx context.Context, _ string) (pan.Account, error) {
			calls.Add(1)
			upstream = ctx
			<-hold
			return pan.Account{ID: "100"}, ctx.Err()
		}}
		d := &Drive{client: client, tokens: testTokens("current")}
		defer d.Close()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		departed, remaining := make(chan error, 1), make(chan error, 1)
		go func() { _, err := d.verifyAccount(ctx, d.snapshot()); departed <- err }()
		synctest.Wait()
		go func() { _, err := d.verifyAccount(t.Context(), d.snapshot()); remaining <- err }()
		synctest.Wait()
		cancel()
		if err := <-departed; !errors.Is(err, context.Canceled) {
			t.Errorf("canceled waiter=%v", err)
		}
		if deadline, ok := upstream.Deadline(); !ok || deadline.Sub(time.Now()) != upstreamTimeout || upstream.Err() != nil {
			t.Error("shared lookup must retain its own bounded context")
		}
		close(hold)
		if err := <-remaining; err != nil || calls.Load() != 1 {
			t.Fatalf("remaining waiter=%v calls=%d", err, calls.Load())
		}
	})
}

func TestSharedAccountLookupTimesOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := &stubClient{account: func(ctx context.Context, _ string) (pan.Account, error) {
			<-ctx.Done()
			return pan.Account{}, ctx.Err()
		}}
		d := &Drive{client: client, tokens: testTokens("current")}
		defer d.Close()
		start := time.Now()
		if _, err := d.verifyAccount(t.Context(), d.snapshot()); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != upstreamTimeout {
			t.Fatalf("elapsed=%v err=%v", time.Since(start), err)
		}
	})
}

func TestCloseWaitsForAccountLookupAndRejectsNewWaiters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hold := make(chan struct{})
		client := &stubClient{account: func(context.Context, string) (pan.Account, error) {
			<-hold
			return pan.Account{ID: "100"}, nil
		}}
		d := &Drive{client: client, tokens: testTokens("current")}
		finished := make(chan error, 1)
		go func() { _, err := d.verifyAccount(t.Context(), d.snapshot()); finished <- err }()
		synctest.Wait()
		closed := make(chan struct{})
		go func() { d.Close(); close(closed) }()
		synctest.Wait()
		select {
		case <-closed:
			t.Error("Close returned while an upstream account request was active")
		default:
		}
		if _, err := d.verifyAccount(t.Context(), d.snapshot()); !errors.Is(err, context.Canceled) {
			t.Errorf("lookup after Close=%v", err)
		}
		close(hold)
		<-closed
		if err := <-finished; !errors.Is(err, context.Canceled) {
			t.Fatalf("lookup completed after Close=%v", err)
		}
	})
}
