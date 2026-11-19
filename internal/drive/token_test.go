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
	d.mu.Lock()
	tokens := d.accountTokens[testSource.AccountID]
	tokens.ExpiresAt = time.Now()
	d.accountTokens[testSource.AccountID] = tokens
	d.mu.Unlock()

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
	if got := d.accountToken(testSource.AccountID).AccessToken; got != "refreshed-access" {
		t.Fatalf("refreshed token not persisted: %q", got)
	}
}
