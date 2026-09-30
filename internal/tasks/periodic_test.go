package tasks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestRunPeriodic_ImmediateExecutionAndTicker(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var count atomic.Int32

	done := make(chan struct{})
	go func() {
		defer close(done)
		RunPeriodic(ctx, logger, "test-job", 20*time.Millisecond, nil, func(ctx context.Context) error {
			if count.Add(1) >= 3 {
				cancel()
			}
			return errors.New("sample error to verify logger handling")
		})
	}()

	select {
	case <-done:
		if count.Load() < 3 {
			t.Fatalf("expected at least 3 executions, got %d", count.Load())
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for periodic runs")
	}
}

func TestRunPeriodic_WakeChannel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var count atomic.Int32
	wake := make(chan struct{}, 5)

	done := make(chan struct{})
	go func() {
		defer close(done)
		RunPeriodic(ctx, logger, "test-wake", 10*time.Second, wake, func(ctx context.Context) error {
			if count.Add(1) == 3 {
				cancel()
			}
			return nil
		})
	}()

	// First execution happens immediately upon starting. Wait briefly then send wake signals.
	time.Sleep(10 * time.Millisecond)
	wake <- struct{}{}
	wake <- struct{}{}

	select {
	case <-done:
		if count.Load() != 3 {
			t.Fatalf("expected exactly 3 executions, got %d", count.Load())
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for wake executions")
	}
}

func TestRunPeriodicSerializesTicksAndWakeups(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		started := make(chan struct{})
		release := make(chan struct{})
		defer close(release)
		wake := make(chan struct{}, 1)
		var calls atomic.Int32
		done := make(chan struct{})
		go func() {
			defer close(done)
			RunPeriodic(ctx, slog.Default(), "serial-job", time.Second, wake, func(context.Context) error {
				if calls.Add(1) == 1 {
					close(started)
					<-release
				} else {
					cancel()
				}
				return nil
			})
		}()
		<-started
		wake <- struct{}{}
		time.Sleep(3 * time.Second)
		synctest.Wait()
		if calls.Load() != 1 {
			t.Fatal("ticks or wakeups started another run while the first was active")
		}
		release <- struct{}{}
		<-done
		if calls.Load() < 2 {
			t.Fatal("pending signals did not trigger a run after the first finished")
		}
	})
}
