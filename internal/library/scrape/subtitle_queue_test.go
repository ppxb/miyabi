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
	q.Enqueue(task1)
	if len(q.tasks) != 1 {
		t.Fatal("expected task1 to be enqueued")
	}

	// 2. Duplicate enqueue for the same movie ID is rejected
	q.Enqueue(task1)
	if len(q.tasks) != 1 {
		t.Fatal("expected duplicate task1 to be rejected")
	}

	// 3. Second unique task fills the buffer (capacity 2)
	q.Enqueue(task2)
	if len(q.tasks) != 2 {
		t.Fatal("expected task2 to be enqueued")
	}

	// 4. Third task exceeds capacity and is dropped cleanly
	q.Enqueue(task3)
	if len(q.tasks) != 2 {
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
	if first, second := <-q.tasks, <-q.tasks; first != task1 || second != task2 {
		t.Fatalf("queued tasks = %+v, %+v", first, second)
	}
}

func TestSubtitleQueue_GracefulShutdown(t *testing.T) {
	service := &Service{}
	q := newSubtitleQueue(service, 2, 8, nil)

	task := SubtitleTask{MovieID: 201}
	q.Enqueue(task)

	// Close waits for active workers to exit.
	q.Close()

	// After close, new tasks must not enter the queue or pending map.
	queued := len(q.tasks)
	taskAfterClose := SubtitleTask{MovieID: 202}
	q.Enqueue(taskAfterClose)
	if len(q.tasks) != queued || q.pending[202] {
		t.Fatal("enqueue after close accepted a task")
	}
}

func TestSubtitleTaskTargetsTheExportedSTRM(t *testing.T) {
	service := &Service{}
	service.SetEmbyExport("emby", "", "")
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
