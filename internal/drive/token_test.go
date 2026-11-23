package drive

import (
	"context"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/pan"
)

func futureTime() time.Time { return time.Now().Add(time.Hour) }

func TestAccountTokenRefresh(t *testing.T) {
	d, client := mountedTestDrive(t)
	// Expire the account token so the next read refreshes it first.
	tokens := testToken(t, d, testSource.AccountID)
	tokens.ExpiresAt = time.Now()
	if err := d.saveAccountTokens(t.Context(), testSource.AccountID, tokens); err != nil {
		t.Fatal(err)
	}

	var used string
	client.list = func(_ context.Context, token, _ string, _, _ int) (pan.FilePage, error) {
		used = token
		return pan.FilePage{}, nil
	}
	sess, err := d.OpenSource(t.Context(), testSource)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.List(t.Context(), "10", 0); err != nil {
		t.Fatal(err)
	}
	if used != "refreshed-access" {
		t.Fatalf("read used token %q, want refreshed token", used)
	}
	if got := testToken(t, d, testSource.AccountID).AccessToken; got != "refreshed-access" {
		t.Fatalf("refreshed token not persisted: %q", got)
	}
}
