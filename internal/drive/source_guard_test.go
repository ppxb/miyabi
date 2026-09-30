package drive

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSourceGuardAllowsDatabaseWritesAndReleasesAfterFailure(t *testing.T) {
	d, _ := mountedTestDrive(t)
	ctx := t.Context()
	session, err := d.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("preparation failed")
	err = session.WithSource(ctx, func() error {
		if d.commit.TryLock() {
			d.commit.Unlock()
			t.Fatal("source changes were not excluded")
		}
		writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := d.database.Movie.Create().SetCode("TEST-001").Exec(writeCtx); err != nil {
			t.Fatalf("source protection opened a database transaction: %v", err)
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("callback error lost: %v", err)
	}
	if !d.commit.TryLock() {
		t.Fatal("source guard leaked after failure")
	}
	waitCtx, cancel := context.WithCancel(ctx)
	cancel()
	err = session.WithSource(waitCtx, func() error { t.Error("canceled callback ran"); return nil })
	d.commit.Unlock()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("guard wait did not honor cancellation: %v", err)
	}
	if err := d.ClearDirectory(ctx); err != nil {
		t.Fatal(err)
	}
	if err := session.WithSource(ctx, func() error { t.Error("unmounted callback ran"); return nil }); !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("unmounted source accepted: %v", err)
	}
}
