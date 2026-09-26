package scrape

import (
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/pan"
)

func TestSubtitleQueue_DeduplicationAndCapacity(t *testing.T) {
	// Create a queue with concurrency 0 (workers don't consume, so tasks stay buffered)
	// We instantiate SubtitleQueue directly for exact queue channel testing.
	q := &SubtitleQueue{
		tasks:   make(chan SubtitleTask, 2),
		pending: make(map[int]bool),
		service: &Service{},
		ctx:     t.Context(),
	}

	task1 := SubtitleTask{MovieID: 101}
	task2 := SubtitleTask{MovieID: 102}
	task3 := SubtitleTask{MovieID: 103}

	// 1. Initial enqueue succeeds
	if !q.Enqueue(task1) {
		t.Fatal("expected task1 to be enqueued")
	}

	// 2. Duplicate enqueue for the same movie ID is rejected
	if q.Enqueue(task1) {
		t.Fatal("expected duplicate task1 to be rejected")
	}

	// 3. Second unique task fills the buffer (capacity 2)
	if !q.Enqueue(task2) {
		t.Fatal("expected task2 to be enqueued")
	}

	// 4. Third task exceeds capacity and is dropped cleanly
	if q.Enqueue(task3) {
		t.Fatal("expected task3 to be dropped due to full queue")
	}

	// Verify that dropped task was removed from pending map
	q.mu.Lock()
	if q.pending[103] {
		t.Fatal("dropped task 103 should not remain in pending map")
	}
	if !q.pending[101] || !q.pending[102] {
		t.Fatal("tasks 101 and 102 should be in pending map")
	}
	q.mu.Unlock()
}

func TestSubtitleQueue_GracefulShutdown(t *testing.T) {
	service := &Service{}
	q := newSubtitleQueue(service, 2, 8, nil)

	task := SubtitleTask{MovieID: 201}
	if !q.Enqueue(task) {
		t.Fatal("expected task to be enqueued")
	}

	// Close waits for workers to drain and exit
	q.Close()

	// After close, enqueue must return false
	taskAfterClose := SubtitleTask{MovieID: 202}
	if q.Enqueue(taskAfterClose) {
		t.Fatal("enqueue after close should return false")
	}
}

func TestSubtitleTaskTargetsTheExportedSTRM(t *testing.T) {
	service := &Service{embyDir: "emby"}
	input := MetadataPayload{MovieID: 7, Code: "SSIS-589"}

	task := service.subtitleTask(input, []pan.File{{ID: "video", Name: "SSIS-589-UC.mp4"}})
	if task == nil || task.MovieID != 7 || task.Target.Dir != filepath.Join("emby", "SSIS", "SSIS-589") ||
		task.Target.Stem != "SSIS-589" || task.Target.Code != "SSIS-589" ||
		!task.Target.Uncensored || !task.Target.HardSubtitled {
		t.Fatalf("subtitle task = %+v", task)
	}
	if task := service.subtitleTask(input, []pan.File{{Name: "SSIS-589.mp4"}}); task == nil || task.Target.Uncensored || task.Target.HardSubtitled {
		t.Fatalf("plain release task = %+v", task)
	}
	parts := []pan.File{{Name: "SSIS-589-CD1.mp4"}, {Name: "SSIS-589-CD2.mp4"}}
	if task := service.subtitleTask(input, parts); task != nil {
		t.Fatalf("multi-part movie received a single subtitle target: %+v", task)
	}
}
