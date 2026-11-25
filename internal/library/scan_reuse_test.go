package library

import (
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

// A scan request only reuses an active scan that covers its work: a queued scan
// takes the latest mode (widening incremental to full), a running scan that
// would skip requested work leaves the request to a new scan.
func TestScanReuseRespectsRequestedMode(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	lib := New(store.Client, nil, tasks.NewService(store.Client, tasks.NewRegistry()), nil, Options{})
	mode := func(t *testing.T, id int) domain.ScanPayload {
		t.Helper()
		payload, err := tasks.DecodePayload[domain.ScanPayload](store.Client.Task.GetX(ctx, id).Payload)
		if err != nil {
			t.Fatal(err)
		}
		return payload
	}
	start := func(t *testing.T, id int) {
		t.Helper()
		store.Client.Task.UpdateOneID(id).SetStatus(task.StatusRunning).ExecX(ctx)
	}
	finish := func(id int) { store.Client.Task.UpdateOneID(id).SetStatus(task.StatusDone).ExecX(ctx) }

	t.Run("queued incremental becomes full", func(t *testing.T) {
		source := domain.LibrarySource{AccountID: "100", Directory: domain.LibraryDirectory{ID: "a"}}
		sync, err := lib.EnqueueSyncMode(ctx, source, false)
		if err != nil {
			t.Fatal(err)
		}
		full, err := lib.EnqueueScan(ctx, source)
		if err != nil || full.ID != sync.ID || mode(t, full.ID).Incremental {
			t.Fatalf("full request on queued sync: id %d (sync %d), payload %+v, err %v", full.ID, sync.ID, mode(t, full.ID), err)
		}
		// A later incremental request must not narrow it back.
		again, err := lib.EnqueueSyncMode(ctx, source, false)
		if err != nil || again.ID != sync.ID || mode(t, sync.ID).Incremental {
			t.Fatalf("incremental request narrowed the queued full scan: %+v %v", mode(t, sync.ID), err)
		}
		finish(sync.ID)
	})

	t.Run("running incremental does not stand in for full", func(t *testing.T) {
		source := domain.LibrarySource{AccountID: "100", Directory: domain.LibraryDirectory{ID: "b"}}
		sync, err := lib.EnqueueSyncMode(ctx, source, false)
		if err != nil {
			t.Fatal(err)
		}
		start(t, sync.ID)
		if reused, err := lib.EnqueueSyncMode(ctx, source, false); err != nil || reused.ID != sync.ID {
			t.Fatalf("incremental request did not reuse running sync: %d %v", reused.ID, err)
		}
		full, err := lib.EnqueueScan(ctx, source)
		if err != nil || full.ID == sync.ID || full.Status != string(task.StatusQueued) || mode(t, full.ID).Incremental {
			t.Fatalf("full request reused running sync: %+v %v", full, err)
		}
		// The running full scan now covers further incremental requests.
		finish(sync.ID)
		start(t, full.ID)
		if reused, err := lib.EnqueueSyncMode(ctx, source, false); err != nil || reused.ID != full.ID {
			t.Fatalf("incremental request did not reuse running full scan: %d %v", reused.ID, err)
		}
		finish(full.ID)
	})

	t.Run("local mode follows the latest setting", func(t *testing.T) {
		source := domain.LibrarySource{AccountID: domain.LocalAccountID, Directory: domain.LibraryDirectory{ID: "c"}}
		local, err := lib.EnqueueScanMode(ctx, source, true)
		if err != nil {
			t.Fatal(err)
		}
		auto, err := lib.EnqueueScanMode(ctx, source, false)
		if err != nil || auto.ID != local.ID || mode(t, local.ID).MetadataOnly {
			t.Fatalf("queued scan kept the old mode: %+v %v", mode(t, local.ID), err)
		}
		start(t, local.ID)
		back, err := lib.EnqueueScanMode(ctx, source, true)
		if err != nil || back.ID == local.ID || !mode(t, back.ID).MetadataOnly {
			t.Fatalf("running scan with another mode was reused: %+v %v", back, err)
		}
	})
}
