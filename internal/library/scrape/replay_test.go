package scrape

import (
	"encoding/json"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestScrapeReplayRecognizesEveryCommittedCoverState(t *testing.T) {
	for _, entry := range []struct {
		name, kind, payload string
		status              task.Status
		committed           bool
	}{
		{"queued", "cover", `{"scrape_task_id":34}`, task.StatusQueued, true},
		{"running", "cover", `{"scrape_task_id":34}`, task.StatusRunning, true},
		{"done", "cover", `{"scrape_task_id":34}`, task.StatusDone, true},
		{"failed", "cover", `{"scrape_task_id":34}`, task.StatusFailed, true},
		{"different parent", "cover", `{"scrape_task_id":35}`, task.StatusQueued, false},
		{"different kind", "scrape", `{"scrape_task_id":34}`, task.StatusQueued, false},
		{"string parent", "cover", `{"scrape_task_id":"34"}`, task.StatusQueued, false},
		{"missing parent", "cover", `{}`, task.StatusQueued, false},
	} {
		t.Run(entry.name, func(t *testing.T) {
			ctx := t.Context()
			store, err := database.Open(ctx, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			cover := store.Client.Task.Create().SetType(entry.kind).SetStatus(entry.status).
				SetPayload(json.RawMessage(entry.payload)).SaveX(ctx)
			// A replay with a committed cover must finish before any drive or
			// catalogue access. Nonmatches reach the missing-drive validation.
			service := &Service{db: store.Client}
			job := tasks.Job{ID: 34, Payload: json.RawMessage(`{"movie_id":1,"code":"TEST-001"}`)}
			for range 2 {
				err := service.Scrape(ctx, job)
				if entry.committed && err != nil || !entry.committed && !domain.IsKind(err, domain.KindInvalid) {
					t.Fatalf("replay committed=%t err=%v", entry.committed, err)
				}
			}
			got := store.Client.Task.Query().OnlyX(ctx)
			if got.ID != cover.ID || got.Status != cover.Status || string(got.Payload) != string(cover.Payload) || !got.UpdatedAt.Equal(cover.UpdatedAt) {
				t.Fatal("replay changed or duplicated the existing task")
			}
		})
	}
}
