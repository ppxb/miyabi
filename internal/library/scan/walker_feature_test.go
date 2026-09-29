package scan

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
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
		fast := New(nil, nil, nil, nil, nil, func(context.Context) error { return nil })
		start = time.Now()
		if err := fast.pace(t.Context()); err != nil || time.Now() != start {
			t.Fatalf("explicit pacing override: %v", err)
		}
	})
}

func TestCheckpointSerializationAndRestoration(t *testing.T) {
	dirs := []Directory{
		{ID: "dir-2", Path: "/Movies/Dir2"},
		{ID: "dir-3", Path: "/Movies/Dir3"},
	}
	bytes, err := json.Marshal(dirs)
	if err != nil {
		t.Fatal(err)
	}

	payload := domain.ScanPayload{
		Checkpoint: string(bytes),
	}

	var restored []Directory
	if payload.Checkpoint != "" {
		if err := json.Unmarshal([]byte(payload.Checkpoint), &restored); err != nil {
			t.Fatal(err)
		}
	}

	if len(restored) != 2 || restored[0].ID != "dir-2" || restored[1].Path != "/Movies/Dir3" {
		t.Fatalf("unexpected restored directories: %+v", restored)
	}
}

func TestDefaultPacing(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	// With cancelled context, DefaultPacing should return ctx.Err()
	err := DefaultPacing(ctx)
	if err == nil {
		t.Fatal("expected context error on timeout, got nil")
	}
}
