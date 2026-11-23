package drive

import (
	"context"
	"errors"
	"testing"

	"github.com/ppxb/miyabi/internal/pan"
)

func TestLoginStoresAccountAndReturnsID(t *testing.T) {
	client := &stubClient{}
	d := newTestDrive(t, client)
	login, err := d.BeginLogin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	status, err := d.LoginStatus(t.Context(), login.ID)
	if err != nil || status.State != pan.LoginAuthorized || status.AccountID != testSource.AccountID {
		t.Fatalf("login status = %+v err=%v", status, err)
	}
	if got := testToken(t, d, testSource.AccountID).AccessToken; got != "login-access" {
		t.Fatalf("login token = %q", got)
	}
}

func TestLoginRejectedDoesNotStoreAccount(t *testing.T) {
	client := &stubClient{
		account: func(_ context.Context, _ string) (pan.Account, error) {
			return pan.Account{}, errors.New("bad token")
		},
	}
	d := newTestDrive(t, client)
	login, err := d.BeginLogin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.LoginStatus(t.Context(), login.ID); err == nil {
		t.Fatal("rejected login returned no error")
	}
	if accounts, err := d.Accounts(t.Context()); err != nil || len(accounts) != 0 {
		t.Fatalf("rejected login stored account: %+v err=%v", accounts, err)
	}
}
