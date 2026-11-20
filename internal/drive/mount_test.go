package drive

import (
	"context"
	"errors"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/pan"
)

func TestSelectAndClearDirectoryPerAccount(t *testing.T) {
	d, _ := mountedTestDrive(t)
	source, ok, err := d.Mount(t.Context(), testSource.AccountID)
	if err != nil || !ok || source != testSource {
		t.Fatalf("mount = %+v ok=%t err=%v", source, ok, err)
	}
	mounts, err := d.Mounts(t.Context())
	if err != nil || len(mounts) != 1 || mounts[0] != testSource {
		t.Fatalf("mounts = %+v err=%v", mounts, err)
	}
	if err := d.ClearDirectoryForAccount(t.Context(), testSource.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := d.Mount(t.Context(), testSource.AccountID); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("clear kept the mount")
	}
	if mounts, err := d.Mounts(t.Context()); err != nil || len(mounts) != 0 {
		t.Fatalf("mounts after clear = %+v err=%v", mounts, err)
	}
}

func TestSelectDirectoryRequiresLogin(t *testing.T) {
	client := &stubClient{}
	d := newTestDrive(t, client)
	_, err := d.SelectDirectoryForAccount(t.Context(), testSource.AccountID, "10")
	if !errors.Is(err, pan.ErrUnauthorized) {
		t.Fatalf("select without login error = %v, want unauthorized", err)
	}
}

func TestSelectDirectoryForAccountIndependent(t *testing.T) {
	client := &stubClient{}
	d := newTestDrive(t, client)
	client.account = func(_ context.Context, token string) (pan.Account, error) {
		if token == "a-access" {
			return pan.Account{ID: "A"}, nil
		}
		return pan.Account{ID: "B"}, nil
	}
	// Log two accounts in (persist tokens into the account registry).
	for _, id := range []string{"A", "B"} {
		tokens := pan.Tokens{AccessToken: string(id) + "-access", RefreshToken: string(id) + "-refresh", ExpiresAt: futureTime()}
		if err := d.saveAccountTokens(t.Context(), id, tokens); err != nil {
			t.Fatal(err)
		}
	}
	selectTestDirectory(t, d, client, "A", domain.LibraryDirectory{ID: "10", Name: "A", Path: "/A"})
	selectTestDirectory(t, d, client, "B", domain.LibraryDirectory{ID: "20", Name: "B", Path: "/B"})
	if source, ok, err := d.Mount(t.Context(), "A"); err != nil || !ok || source.Directory.ID != "10" {
		t.Fatalf("account A mount = %+v ok=%t err=%v", source, ok, err)
	}
	if source, ok, err := d.Mount(t.Context(), "B"); err != nil || !ok || source.Directory.ID != "20" {
		t.Fatalf("account B mount = %+v ok=%t err=%v", source, ok, err)
	}
}
