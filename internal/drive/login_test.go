package drive

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/pan"
)

// testPollWindow shortens a long poll so a test does not wait one out.
func testPollWindow(t *testing.T, window time.Duration) {
	t.Helper()
	previous := loginPollWindow
	loginPollWindow = window
	t.Cleanup(func() { loginPollWindow = previous })
}

func TestLateQRCodeCannotRestoreReplacedLogin(t *testing.T) {
	for _, action := range []string{"disconnect", "login"} {
		t.Run(action, func(t *testing.T) {
			d, client := mountedTestDrive(t)
			started := make(chan struct{})
			hold, release := testGate(t)
			var calls atomic.Int32
			client.beginLogin = func(context.Context) (*pan.Login, error) {
				if calls.Add(1) == 1 {
					close(started)
					<-hold
				}
				return &pan.Login{QRCode: []byte("qr")}, nil
			}
			finished := make(chan error, 1)
			go func() {
				_, err := d.BeginLogin(t.Context())
				finished <- err
			}()
			await(t, started)
			var current LoginSession
			if action == "disconnect" {
				if _, err := d.Disconnect(t.Context()); err != nil {
					t.Fatal(err)
				}
			} else {
				var err error
				current, err = d.BeginLogin(t.Context())
				if err != nil {
					t.Fatal(err)
				}
			}
			release()
			var conflict *domain.Error
			if err := await(t, finished); !errors.As(err, &conflict) || conflict.Kind != domain.KindConflict {
				t.Fatalf("replaced QR request = %v, want conflict", err)
			}
			if action == "login" {
				if status, err := d.LoginStatus(t.Context(), current.ID); err != nil || status.State != pan.LoginAuthorized {
					t.Fatalf("replacement login = %+v, %v", status, err)
				}
			} else if status, err := d.Account(t.Context()); err != nil || status.Connected {
				t.Fatalf("account after disconnect = %+v, %v", status, err)
			}
		})
	}
}

func TestLoginPollsShareExchangeAfterCallerCancellation(t *testing.T) {
	d, client := mountedTestDrive(t)
	tokens := testTokens("login")
	const waiters = 8
	started := make(chan struct{}, waiters+1)
	hold, release := testGate(t)
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
	login, err := d.BeginLogin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	departed := make(chan error, 1)
	go func() {
		_, err := d.LoginStatus(ctx, login.ID)
		departed <- err
	}()
	await(t, started)
	finished := make(chan error, waiters)
	for range waiters {
		go func() {
			status, err := d.LoginStatus(t.Context(), login.ID)
			if err == nil && status.State != pan.LoginAuthorized {
				err = fmt.Errorf("login state = %s", status.State)
			}
			finished <- err
		}()
	}
	cancel()
	if err := await(t, departed); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled login waiter = %v", err)
	}
	release()
	for range waiters {
		if err := await(t, finished); err != nil {
			t.Fatal(err)
		}
	}
	if polls.Load() != 1 || exchanges.Load() != 1 {
		t.Fatalf("shared login polled %d times and exchanged %d times", polls.Load(), exchanges.Load())
	}
	assertTokens(t, d, tokens)
}

func TestLateLoginExchangeCannotReplaceCurrentSession(t *testing.T) {
	for _, action := range []string{"disconnect", "login"} {
		t.Run(action, func(t *testing.T) {
			d, client := mountedTestDrive(t)
			newTokens := testTokens("current-login")
			oldLogin := &pan.Login{QRCode: []byte("old")}
			var logins atomic.Int32
			client.beginLogin = func(context.Context) (*pan.Login, error) {
				if logins.Add(1) == 1 {
					return oldLogin, nil
				}
				return &pan.Login{QRCode: []byte("new")}, nil
			}
			started := make(chan struct{}, 1)
			hold, release := testGate(t)
			client.exchangeToken = func(_ context.Context, login *pan.Login) (pan.Tokens, error) {
				if login == oldLogin {
					started <- struct{}{}
					<-hold
					return testTokens("stale-login"), nil
				}
				return newTokens, nil
			}
			old, err := d.BeginLogin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			finished := make(chan error, 1)
			go func() {
				status, err := d.LoginStatus(t.Context(), old.ID)
				if err == nil && status.State != pan.LoginExpired {
					err = fmt.Errorf("old login state = %s", status.State)
				}
				finished <- err
			}()
			await(t, started)
			var want pan.Tokens
			if action == "disconnect" {
				if _, err := d.Disconnect(t.Context()); err != nil {
					t.Fatal(err)
				}
			} else {
				want = newTokens
				current, err := d.BeginLogin(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if status, err := d.LoginStatus(t.Context(), current.ID); err != nil || status.State != pan.LoginAuthorized {
					t.Fatalf("current login = %+v, %v", status, err)
				}
			}
			release()
			if err := await(t, finished); err != nil {
				t.Fatal(err)
			}
			assertTokens(t, d, want)
		})
	}
}

// A long poll that runs out our own window carries no news about the login, and
// the user is usually still lining the QR code up.
func TestLoginPollWindowElapsingIsNotAFailure(t *testing.T) {
	d, client := mountedTestDrive(t)
	testPollWindow(t, 20*time.Millisecond)
	var polls atomic.Int32
	client.loginStatus = func(ctx context.Context, _ *pan.Login) (pan.LoginState, error) {
		if polls.Add(1) == 1 {
			<-ctx.Done()
			return "", ctx.Err()
		}
		return pan.LoginScanned, nil
	}
	login, err := d.BeginLogin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status, err := d.LoginStatus(t.Context(), login.ID); err != nil || status.State != pan.LoginWaiting {
		t.Fatalf("elapsed poll window = %+v, %v", status, err)
	}
	if status, err := d.LoginStatus(t.Context(), login.ID); err != nil || status.State != pan.LoginScanned {
		t.Fatalf("poll after the window elapsed = %+v, %v", status, err)
	}
}

// The caller owns the retry budget, so a transport failure must leave the session
// usable rather than writing the failure into it.
func TestLoginTransportFailureKeepsTheSession(t *testing.T) {
	d, client := mountedTestDrive(t)
	var polls atomic.Int32
	client.loginStatus = func(context.Context, *pan.Login) (pan.LoginState, error) {
		if polls.Add(1) == 1 {
			return "", errors.New("connection reset by peer")
		}
		return pan.LoginScanned, nil
	}
	login, err := d.BeginLogin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.LoginStatus(t.Context(), login.ID); err == nil {
		t.Fatal("transport failure was reported as a login state")
	}
	if status, err := d.LoginStatus(t.Context(), login.ID); err != nil || status.State != pan.LoginScanned {
		t.Fatalf("poll after a transport failure = %+v, %v", status, err)
	}
}

// Scanning is progress: a poll that brings no news, whether 115 answers "waiting"
// or our own window runs out first, must not walk the dialog back from "confirm
// on your phone" to "waiting for a scan".
func TestLoginKeepsScannedWhenAPollBringsNoNews(t *testing.T) {
	for _, test := range []struct {
		name   string
		noNews func(context.Context) (pan.LoginState, error)
	}{
		{"waiting answer", func(context.Context) (pan.LoginState, error) { return pan.LoginWaiting, nil }},
		{"elapsed window", func(ctx context.Context) (pan.LoginState, error) { <-ctx.Done(); return "", ctx.Err() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			d, client := mountedTestDrive(t)
			testPollWindow(t, 20*time.Millisecond)
			var polls atomic.Int32
			client.loginStatus = func(ctx context.Context, _ *pan.Login) (pan.LoginState, error) {
				if polls.Add(1) == 1 {
					return pan.LoginScanned, nil
				}
				return test.noNews(ctx)
			}
			login, err := d.BeginLogin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if status, err := d.LoginStatus(t.Context(), login.ID); err != nil || status.State != pan.LoginScanned {
				t.Fatalf("scan = %+v, %v", status, err)
			}
			if status, err := d.LoginStatus(t.Context(), login.ID); err != nil || status.State != pan.LoginScanned {
				t.Fatalf("poll with no news = %+v, %v", status, err)
			}
		})
	}
}

// 115 does not always report a dead QR, so a login has to expire on its own.
func TestLoginExpiresAfterItsLifetime(t *testing.T) {
	d, client := mountedTestDrive(t)
	client.loginStatus = func(context.Context, *pan.Login) (pan.LoginState, error) {
		return pan.LoginWaiting, nil
	}
	login, err := d.BeginLogin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	d.login.session.startedAt = time.Now().Add(-loginLifetime - time.Minute)
	d.mu.Unlock()
	if status, err := d.LoginStatus(t.Context(), login.ID); err != nil || status.State != pan.LoginExpired {
		t.Fatalf("stale login = %+v, %v", status, err)
	}
	if status, err := d.LoginStatus(t.Context(), login.ID); err != nil || status.State != pan.LoginExpired {
		t.Fatalf("stale login polled again = %+v, %v", status, err)
	}
}

func TestAccountVerificationIsCachedBriefly(t *testing.T) {
	d, client := mountedTestDrive(t)
	var calls atomic.Int32
	client.account = func(context.Context, string) (pan.Account, error) {
		calls.Add(1)
		return pan.Account{ID: testSource.AccountID}, nil
	}
	d.invalidateAccountCache()
	for range 3 {
		status, err := d.Account(t.Context())
		if err != nil || !status.Connected || status.Directory == nil || status.Directory.ID != testSource.Directory.ID {
			t.Fatalf("account = %+v, %v", status, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("account verified upstream %d times within the cache window", calls.Load())
	}
	if _, err := d.Disconnect(t.Context()); err != nil {
		t.Fatal(err)
	}
	if status, err := d.Account(t.Context()); err != nil || status.Connected {
		t.Fatalf("account after disconnect = %+v, %v", status, err)
	}
	loginTestAccount(t, d)
	if _, err := d.Account(t.Context()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("a new login reused the previous account verification: %d calls", calls.Load())
	}
}

func TestInFlightAccountFetchDoesNotPoisonCacheAfterRelogin(t *testing.T) {
	d, client := mountedTestDrive(t)
	oldAccount := pan.Account{ID: testSource.AccountID, Name: "OldUser"}
	newAccount := pan.Account{ID: "200", Name: "NewUser"}

	fetchGate, releaseFetch := testGate(t)
	fetchStarted := make(chan struct{}, 1)

	client.account = func(ctx context.Context, token string) (pan.Account, error) {
		if token != "new-token" {
			select {
			case fetchStarted <- struct{}{}:
			default:
			}
			<-fetchGate
			return oldAccount, nil
		}
		return newAccount, nil
	}

	d.invalidateAccountCache()
	state1 := d.snapshot()

	fetchDone := make(chan error, 1)
	go func() {
		_, err := d.verifyAccount(t.Context(), state1)
		fetchDone <- err
	}()

	await(t, fetchStarted)

	newTokens := pan.Tokens{AccessToken: "new-token", RefreshToken: "new-refresh", ExpiresAt: time.Now().Add(time.Hour)}
	if err := database.SaveSetting(t.Context(), d.database, credentialsSetting, newTokens); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	d.tokens = newTokens
	d.credentialVersion++
	d.tokenVersion++
	d.authorizationVersion++
	d.mu.Unlock()
	d.invalidateAccountCache()

	// The new login must not wait for or join the old credential's lookup.
	state2 := d.snapshot()
	acc2, err := d.verifyAccount(t.Context(), state2)
	if err != nil {
		t.Fatalf("verifyAccount for state2 failed: %v", err)
	}
	if acc2.ID != newAccount.ID {
		t.Fatalf("verifyAccount returned old account %s, want new account %s", acc2.ID, newAccount.ID)
	}
	releaseFetch()
	if err := await(t, fetchDone); !errors.Is(err, pan.ErrUnauthorized) {
		t.Fatalf("in-flight verifyAccount error = %v, want pan.ErrUnauthorized", err)
	}

	d.accountCacheMu.Lock()
	cachedID := d.cachedAccount.ID
	cachedVer := d.cachedCredentialVersion
	d.accountCacheMu.Unlock()

	if cachedID != newAccount.ID || cachedVer != state2.credentialVersion {
		t.Fatalf("cache poisoned: got ID=%s Ver=%d, want ID=%s Ver=%d", cachedID, cachedVer, newAccount.ID, state2.credentialVersion)
	}
}
