package drive

import "testing"

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

func TestDisconnectAccountRemovesMount(t *testing.T) {
	d, _ := mountedTestDrive(t)
	if err := d.DisconnectAccount(t.Context(), testSource.AccountID); err != nil {
		t.Fatal(err)
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
