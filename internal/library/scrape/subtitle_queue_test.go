package scrape

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"testing/synctest"

	subtitlemeta "github.com/ppxb/miyabi/internal/domain/subtitle"
	"github.com/ppxb/miyabi/internal/pan"
)

type subtitleExporterFunc func(context.Context, int, subtitlemeta.Target) (int, error)

func (f subtitleExporterFunc) Export(ctx context.Context, movieID int, target subtitlemeta.Target) (int, error) {
	return f(ctx, movieID, target)
}

func TestSubtitleQueue_DeduplicationAndCapacity(t *testing.T) {
	// Leave workers out so queue capacity and deduplication are deterministic.
	q := &SubtitleQueue{
		tasks:   make(chan SubtitleTask, 2),
		pending: make(map[int]bool),
		logger:  slog.Default(),
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
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		cancelled := make(chan struct{})
		release := make(chan struct{})
		defer close(release)
		exporter := subtitleExporterFunc(func(ctx context.Context, _ int, _ subtitlemeta.Target) (int, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			<-release
			return 0, ctx.Err()
		})
		service := New(nil, nil, nil, nil, nil, Dependencies{Subtitles: exporter})
		t.Cleanup(service.Close)
		q := service.subtitleQueue
		q.Enqueue(SubtitleTask{MovieID: 201})
		<-started

		closed := make(chan struct{})
		go func() {
			service.Close()
			close(closed)
		}()
		<-cancelled
		synctest.Wait()
		select {
		case <-closed:
			t.Fatal("close returned before the active exporter finished")
		default:
		}
		release <- struct{}{}
		<-closed
		if q.pending[201] {
			t.Fatal("finished task remains pending after close")
		}

		// After close, new tasks must not enter the queue or pending map.
		queued := len(q.tasks)
		q.Enqueue(SubtitleTask{MovieID: 202})
		if len(q.tasks) != queued || q.pending[202] {
			t.Fatal("enqueue after close accepted a task")
		}
	})
}

func TestSubtitleTaskTargetsTheExportedSTRM(t *testing.T) {
	service := &Service{subtitles: subtitleExporterFunc(func(context.Context, int, subtitlemeta.Target) (int, error) {
		return 0, nil
	})}
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
	service.subtitles = nil
	if task := service.subtitleTask(input, []pan.File{{Name: "SSIS-589.mp4"}}); task != nil {
		t.Fatalf("subtitle task created without an exporter: %+v", task)
	}
}
