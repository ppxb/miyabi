package service

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/ppxb/miyabi/internal/pan"
)

func TestPanLoginPollsShareExchangeAfterCallerCancellation(t *testing.T) {
	library, client := panConcurrencyFixture(t)
	drive, tokens := library.drive, panTestTokens("login")
	const waiters = 8
	started := make(chan struct{}, waiters+1)
	hold, release := panTestGate(t)
	var polls, exchanges atomic.Int32
	client.loginStatus = func(context.Context, *pan.Login) (pan.LoginState, error) {
		polls.Add(1)
		return pan.LoginAuthorized, nil
	}
	client.exchangeToken = func(ctx context.Context, _ *pan.Login) (pan.Tokens, error) {
		exchanges.Add(1)
		started <- struct{}{}
		<-hold
		return tokens, ctx.Err()
	}
	login, err := drive.BeginLogin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	departed := make(chan error, 1)
	go func() {
		_, err := drive.LoginStatus(ctx, login.ID)
		departed <- err
	}()
	awaitPan(t, started)
	finished := make(chan error, waiters)
	for range waiters {
		go func() {
			status, err := drive.LoginStatus(t.Context(), login.ID)
			if err == nil && status.State != pan.LoginAuthorized {
				err = fmt.Errorf("login state = %s", status.State)
			}
			finished <- err
		}()
	}
	cancel()
	if err := awaitPan(t, departed); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled login waiter = %v", err)
	}
	release()
	for range waiters {
		if err := awaitPan(t, finished); err != nil {
			t.Fatal(err)
		}
	}
	if polls.Load() != 1 || exchanges.Load() != 1 {
		t.Fatalf("shared login polled %d times and exchanged %d times", polls.Load(), exchanges.Load())
	}
	assertPanTokens(t, drive, tokens)
}

func TestPanLateLoginExchangeCannotReplaceCurrentSession(t *testing.T) {
	for _, action := range []string{"disconnect", "login"} {
		t.Run(action, func(t *testing.T) {
			library, client := panConcurrencyFixture(t)
			drive, newTokens := library.drive, panTestTokens("current-login")
			oldLogin := &pan.Login{QRCode: []byte("old")}
			var logins atomic.Int32
			client.beginLogin = func(context.Context) (*pan.Login, error) {
				if logins.Add(1) == 1 {
					return oldLogin, nil
				}
				return &pan.Login{QRCode: []byte("new")}, nil
			}
			started := make(chan struct{}, 1)
			hold, release := panTestGate(t)
			client.exchangeToken = func(_ context.Context, login *pan.Login) (pan.Tokens, error) {
				if login == oldLogin {
					started <- struct{}{}
					<-hold
					return panTestTokens("stale-login"), nil
				}
				return newTokens, nil
			}
			old, err := drive.BeginLogin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			finished := make(chan error, 1)
			go func() {
				status, err := drive.LoginStatus(t.Context(), old.ID)
				if err == nil && status.State != pan.LoginExpired {
					err = fmt.Errorf("old login state = %s", status.State)
				}
				finished <- err
			}()
			awaitPan(t, started)
			var want pan.Tokens
			if action == "disconnect" {
				if _, err := drive.Disconnect(t.Context()); err != nil {
					t.Fatal(err)
				}
			} else {
				want = newTokens
				current, err := drive.BeginLogin(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if status, err := drive.LoginStatus(t.Context(), current.ID); err != nil || status.State != pan.LoginAuthorized {
					t.Fatalf("current login = %+v, %v", status, err)
				}
			}
			release()
			if err := awaitPan(t, finished); err != nil {
				t.Fatal(err)
			}
			assertPanTokens(t, drive, want)
		})
	}
}
