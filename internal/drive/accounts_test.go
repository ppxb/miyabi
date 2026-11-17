package drive

import (
	"context"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/pan"
)

func TestAccountRegistryAndMount(t *testing.T) {
	ctx := t.Context()
	d := newTestDrive(t, &stubClient{})
	tokens := testTokens("account")
	if err := d.saveAccountTokens(ctx, "A", tokens); err != nil {
		t.Fatal(err)
	}
	if got := d.accountToken("A"); got.AccessToken != tokens.AccessToken {
		t.Fatalf("cached token = %+v", got)
	}

	mount := mountRecord{
		AccountID:        "A",
		LibraryDirectory: domain.LibraryDirectory{ID: "10", Name: "Movies", Path: "/Movies"},
	}
	if err := d.saveAccountMount(ctx, "A", mount); err != nil {
		t.Fatal(err)
	}
	if source, ok := d.Mount(ctx, "A"); !ok || source.Directory.ID != "10" || source.AccountID != "A" {
		t.Fatalf("mount = %+v, ok = %t", source, ok)
	}

	if err := d.removeAccount(ctx, "A"); err != nil {
		t.Fatal(err)
	}
	if d.accountToken("A").AccessToken != "" {
		t.Fatal("account token was not cleared")
	}
	if _, ok := d.Mount(ctx, "A"); ok {
		t.Fatal("account mount was not cleared")
	}
}

func TestOpenSourceForRegisteredAccount(t *testing.T) {
	ctx := t.Context()
	client := &stubClient{}
	d := newTestDrive(t, client)
	if err := d.saveAccountTokens(ctx, "A", testTokens("account")); err != nil {
		t.Fatal(err)
	}
	mount := mountRecord{
		AccountID:        "A",
		LibraryDirectory: domain.LibraryDirectory{ID: "10", Name: "Movies", Path: "/Movies"},
	}
	if err := d.saveAccountMount(ctx, "A", mount); err != nil {
		t.Fatal(err)
	}

	session, err := d.OpenSource(ctx, mount.source())
	if err != nil {
		t.Fatal(err)
	}
	if session.Source().AccountID != "A" || session.Source().Directory.ID != "10" {
		t.Fatalf("session source = %+v", session.Source())
	}
	if _, err := session.List(ctx, "10", 0); err != nil {
		t.Fatalf("list through account session: %v", err)
	}
}

func TestRefreshAccountTokens(t *testing.T) {
	ctx := t.Context()
	client := &stubClient{
		refreshToken: func(context.Context, string) (pan.Tokens, error) {
			return testTokens("rotated"), nil
		},
	}
	d := newTestDrive(t, client)
	if err := d.saveAccountTokens(ctx, "A", pan.Tokens{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	if err := d.refreshAccountTokens(ctx, "A"); err != nil {
		t.Fatal(err)
	}
	if got := d.accountToken("A"); got.AccessToken != "rotated-access" {
		t.Fatalf("refreshed token = %+v", got)
	}
}

func TestAccountsRefreshesExpiredTokens(t *testing.T) {
	ctx := t.Context()
	client := &stubClient{
		refreshToken: func(context.Context, string) (pan.Tokens, error) { return testTokens("rotated"), nil },
		account:      func(context.Context, string) (pan.Account, error) { return pan.Account{ID: "A", Name: "acc"}, nil },
	}
	d := newTestDrive(t, client)
	if err := d.saveAccountTokens(ctx, "A", pan.Tokens{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	accounts, err := d.Accounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || !accounts[0].Connected {
		t.Fatalf("accounts = %+v", accounts)
	}
	if got := d.accountToken("A"); got.AccessToken != "rotated-access" {
		t.Fatalf("refreshed token = %+v", got)
	}
}

func TestRefreshExpiringAccountsSkipsFreshTokens(t *testing.T) {
	ctx := t.Context()
	called := false
	client := &stubClient{
		refreshToken: func(context.Context, string) (pan.Tokens, error) {
			called = true
			return testTokens("rotated"), nil
		},
	}
	d := newTestDrive(t, client)
	if err := d.saveAccountTokens(ctx, "A", testTokens("fresh")); err != nil {
		t.Fatal(err)
	}

	d.refreshExpiringAccounts(ctx)
	if called {
		t.Fatal("fresh token should not be refreshed")
	}
}
