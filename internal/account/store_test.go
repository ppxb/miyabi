package account

import (
	"context"
	"errors"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
)

func TestStorePersistsAndVerifiesCredentials(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	accounts, err := New(ctx, store.Client)
	if err != nil {
		t.Fatal(err)
	}
	if accounts.Configured() {
		t.Fatal("fresh store must not be configured")
	}
	if err := accounts.Set(ctx, " admin ", "password123"); err != nil {
		t.Fatal(err)
	}
	if accounts.Username() != "admin" || !accounts.Configured() {
		t.Fatalf("unexpected account: %#v", accounts)
	}
	if err := accounts.Verify("admin", "password123"); err != nil {
		t.Fatalf("valid credentials rejected: %v", err)
	}
	if err := accounts.Verify("admin", "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v", err)
	}
	if err := accounts.Verify("other", "password123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong username error = %v", err)
	}

	reloaded, err := New(ctx, store.Client)
	if err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Verify("admin", "password123"); err != nil {
		t.Fatalf("persisted credentials rejected: %v", err)
	}
}

func TestStoreRejectsEmptyUsernameAndShortPassword(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	accounts, err := New(ctx, store.Client)
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.Set(ctx, " ", "password123"); err == nil {
		t.Fatal("expected empty username error")
	}
	if err := accounts.Set(ctx, "admin", "short"); err == nil {
		t.Fatal("expected short password error")
	}
}
