package drive

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/pan"
)

func TestAccountsAndStatus(t *testing.T) {
	d, _ := mountedTestDrive(t)
	accounts, err := d.Accounts(t.Context())
	if err != nil || len(accounts) != 1 {
		t.Fatalf("accounts = %+v err=%v", accounts, err)
	}
	if !accounts[0].Connected || accounts[0].Account == nil || accounts[0].Account.ID != testSource.AccountID {
		t.Fatalf("account status = %+v", accounts[0])
	}
	if accounts[0].Directory == nil || accounts[0].Directory.ID != testSource.Directory.ID {
		t.Fatalf("account directory = %+v", accounts[0].Directory)
	}
	status, err := d.AccountStatusFor(t.Context(), testSource.AccountID)
	if err != nil || !status.Connected || status.Account == nil || status.Account.ID != testSource.AccountID {
		t.Fatalf("status = %+v err=%v", status, err)
	}
}

func TestAccountStatusForCachesProfile(t *testing.T) {
	ctx := t.Context()
	client := &stubClient{}
	d := newTestDrive(t, client)
	mountTestSource(t, d, client, testSource)

	calls := 0
	client.account = func(context.Context, string) (pan.Account, error) {
		calls++
		return pan.Account{ID: testSource.AccountID, Name: "cached"}, nil
	}
	base := time.Now()
	d.now = func() time.Time { return base }

	first, err := d.AccountStatusFor(ctx, testSource.AccountID)
	if err != nil || calls != 1 || first.Directory == nil {
		t.Fatalf("first status = %+v calls=%d err=%v", first, calls, err)
	}
	if _, err := d.AccountStatusFor(ctx, testSource.AccountID); err != nil || calls != 1 {
		t.Fatalf("cached status calls = %d err=%v", calls, err)
	}

	d.now = func() time.Time { return base.Add(accountStatusTTL + time.Second) }
	if _, err := d.AccountStatusFor(ctx, testSource.AccountID); err != nil || calls != 2 {
		t.Fatalf("expired status calls = %d err=%v", calls, err)
	}

	// Clearing the mount must not leave the old directory in the cached profile.
	if err := d.ClearDirectoryForAccount(ctx, testSource.AccountID); err != nil {
		t.Fatal(err)
	}
	status, err := d.AccountStatusFor(ctx, testSource.AccountID)
	if err != nil || calls != 3 || status.Directory != nil {
		t.Fatalf("invalidated status = %+v calls=%d err=%v", status, calls, err)
	}
}

func TestStoreAccountStatusDropsStaleGeneration(t *testing.T) {
	d := newTestDrive(t, &stubClient{})
	generation := d.accountStatusCacheGeneration()
	d.invalidateAccountStatus("A")
	d.storeAccountStatus("A", AccountStatus{AccountID: "A", Connected: true}, generation)
	if _, ok := d.cachedAccountStatus("A"); ok {
		t.Fatal("status from before an invalidation was cached")
	}
}

func TestDisconnectAccountRemovesMount(t *testing.T) {
	d, _ := mountedTestDrive(t)
	// Cache the profile so this also proves disconnect invalidates it.
	if status, err := d.AccountStatusFor(t.Context(), testSource.AccountID); err != nil || !status.Connected {
		t.Fatalf("status before disconnect = %+v err=%v", status, err)
	}
	if err := d.DisconnectAccount(t.Context(), testSource.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AccountStatusFor(t.Context(), testSource.AccountID); !errors.Is(err, pan.ErrUnauthorized) {
		t.Fatalf("status after disconnect: %v", err)
	}
	if _, ok, err := d.Mount(t.Context(), testSource.AccountID); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("disconnect kept the mount")
	}
	if accounts, err := d.Accounts(t.Context()); err != nil || len(accounts) != 0 {
		t.Fatalf("accounts after disconnect = %+v err=%v", accounts, err)
	}
}
