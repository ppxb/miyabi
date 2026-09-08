package service

import (
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/pan"
)

func TestOfflineCompletionRecordsTheFileWithoutCreatingAMovie(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	record, err := store.Client.Task.Create().SetType("offline").SetStatus(task.StatusRunning).
		SetPayload(map[string]any{"code": "ABP-001"}).Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	service := &OfflineService{database: store.Client}
	if err := service.updateTask(t.Context(), record, pan.OfflineTask{Status: 2, Progress: 100, FileID: "42"}); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Client.Task.Get(t.Context(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != task.StatusDone || updated.Progress != 100 || updated.Payload["file_id"] != "42" {
		t.Fatalf("completed task = %#v", updated)
	}
	count, err := store.Client.Movie.Query().Count(t.Context())
	if err != nil || count != 0 {
		t.Fatalf("movie count = %d, error = %v", count, err)
	}
}

func TestOfflineTasksReturnLatestAttemptForCurrentMovieAndAccount(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hashA, hashB := strings.Repeat("a", 40), strings.Repeat("b", 40)
	for _, fixture := range []struct {
		movieID   string
		accountID string
		hash      string
		status    task.Status
	}{
		{"movie", "100", hashA, task.StatusFailed},
		{"movie", "100", hashA, task.StatusDone},
		{"movie", "100", hashB, task.StatusRunning},
		{"other", "100", hashA, task.StatusRunning},
		{"movie", "200", hashA, task.StatusRunning},
	} {
		if err := store.Client.Task.Create().SetType("offline").SetStatus(fixture.status).
			SetPayload(map[string]any{
				"javdb_id": fixture.movieID, "account_id": fixture.accountID, "hash": fixture.hash,
			}).Exec(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	service := &OfflineService{database: store.Client}
	records, err := service.Tasks(t.Context(), "movie", "100")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Hash != hashB || records[0].Status != task.StatusRunning ||
		records[1].Hash != hashA || records[1].Status != task.StatusDone {
		t.Fatalf("offline tasks = %#v", records)
	}
	empty, err := service.Tasks(t.Context(), "missing", "100")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty tasks = %#v, error = %v", empty, err)
	}
}
