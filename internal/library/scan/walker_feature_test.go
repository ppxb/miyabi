package scan

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestCheckpointSerializationAndRestoration(t *testing.T) {
	dirs := []Directory{
		{ID: "dir-2", Path: "/Movies/Dir2"},
		{ID: "dir-3", Path: "/Movies/Dir3"},
	}
	bytes, err := json.Marshal(dirs)
	if err != nil {
		t.Fatal(err)
	}

	payload := Payload{
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
