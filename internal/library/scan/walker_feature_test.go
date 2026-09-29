package scan

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestScannerDefaultPacingAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		scanner := New(nil, nil, nil, nil, nil, nil)
		start := time.Now()
		if err := scanner.pace(t.Context()); err != nil {
			t.Fatal(err)
		}
		if !time.Now().After(start) {
			t.Fatal("default scanner did not throttle requests")
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := scanner.pace(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("pacing cancellation: %v", err)
		}
		deadline, stop := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer stop()
		if err := scanner.pace(deadline); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("pacing deadline: %v", err)
		}
		fast := New(nil, nil, nil, nil, nil, func(context.Context) error { return nil })
		start = time.Now()
		if err := fast.pace(t.Context()); err != nil || time.Now() != start {
			t.Fatalf("explicit pacing override: %v", err)
		}
	})
}
