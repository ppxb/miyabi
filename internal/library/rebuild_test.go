package library

import (
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestRebuildQueuesFreshScanAndDeduplicatesRepeatedRequests(t *testing.T) {
	lib, ordinary, source := libraryFixture(t)
	ctx := t.Context()
	first, err := lib.StartRebuild(ctx)
	if err != nil || !first.Rebuild || first.ID == ordinary.ID || first.Source != source.Source {
		t.Fatalf("rebuild reused an ordinary scan: %+v %v", first, err)
	}
	for _, status := range []task.Status{task.StatusQueued, task.StatusRunning} {
		lib.database.Task.UpdateOneID(first.ID).SetStatus(status).ExecX(ctx)
		again, err := lib.StartRebuild(ctx)
		if err != nil || again.ID != first.ID || !again.Rebuild {
			t.Fatalf("duplicate rebuild: %+v %v", again, err)
		}
	}
	input, err := tasks.DecodePayload[domain.ScanPayload](lib.database.Task.GetX(ctx, first.ID).Payload)
	if err != nil || !input.Rebuild || input.TargetID != "" {
		t.Fatalf("rebuild does not cover the full mount: %+v %v", input, err)
	}
	if err := lib.drive.ClearDirectory(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.StartRebuild(ctx); err == nil {
		t.Fatal("rebuild accepted an unmounted source")
	}
}
