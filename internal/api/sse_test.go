package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/tasks"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/ent/task"
)

// sseTaskStub is a TaskManager whose snapshot and revisions change when the
// test publishes an update, mirroring TaskService's coalesced notifications.
type sseTaskStub struct {
	mu          sync.Mutex
	updates     chan struct{}
	revisions   tasks.TaskRevisions
	progress    int
	unsubscribe chan struct{}
}

func newSSETaskStub() *sseTaskStub {
	return &sseTaskStub{updates: make(chan struct{}, 1), unsubscribe: make(chan struct{}, 1)}
}

func (stub *sseTaskStub) Subscribe() (<-chan struct{}, func()) {
	return stub.updates, func() {
		select {
		case stub.unsubscribe <- struct{}{}:
		default:
		}
	}
}

func (stub *sseTaskStub) List(context.Context) ([]tasks.TaskInfo, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return []tasks.TaskInfo{{ID: 1, Type: "scan", Status: task.StatusRunning, Progress: stub.progress,
		Scan: domain.ScanProgress{Stage: "scanning"}}}, nil
}

func (stub *sseTaskStub) Revisions() tasks.TaskRevisions {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return stub.revisions
}

func (stub *sseTaskStub) publish(progress int) {
	stub.mu.Lock()
	stub.progress = progress
	stub.revisions.Library++
	stub.mu.Unlock()
	select {
	case stub.updates <- struct{}{}:
	default:
	}
}

type sseEvent struct {
	name string
	data string
}

// readEvent parses one server-sent event; gin writes "event:" then "data:".
func readEvent(t *testing.T, reader *bufio.Reader) sseEvent {
	t.Helper()
	var event sseEvent
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE stream: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			if event.name != "" {
				return event
			}
		case strings.HasPrefix(line, "event:"):
			event.name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			event.data += strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}
}

func TestTaskEventsStreamsSnapshotsAndRevisionsUntilTheClientLeaves(t *testing.T) {
	stub := newSSETaskStub()
	server := httptest.NewServer(NewRouter(Dependencies{Tasks: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/tasks/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("response = %d %s", response.StatusCode, response.Header.Get("Content-Type"))
	}
	if cacheControl := response.Header.Get("Cache-Control"); !strings.Contains(cacheControl, "no-cache") {
		t.Fatalf("Cache-Control = %q", cacheControl)
	}
	reader := bufio.NewReader(response.Body)

	expectSnapshot := func(progress int, revision uint64) {
		t.Helper()
		event := readEvent(t, reader)
		if event.name != "tasks" {
			t.Fatalf("event = %+v, want tasks", event)
		}
		var infos []tasks.TaskInfo
		if err := json.Unmarshal([]byte(event.data), &infos); err != nil || len(infos) != 1 || infos[0].Progress != progress {
			t.Fatalf("tasks payload = %s (%v), want progress %d", event.data, err, progress)
		}
		changes := readEvent(t, reader)
		if changes.name != "changes" {
			t.Fatalf("event = %+v, want changes", changes)
		}
		var revisions tasks.TaskRevisions
		if err := json.Unmarshal([]byte(changes.data), &revisions); err != nil || revisions.Library != revision {
			t.Fatalf("changes payload = %s (%v), want library revision %d", changes.data, err, revision)
		}
	}
	// The stream opens with a snapshot even when nothing changed yet.
	expectSnapshot(0, 0)
	stub.publish(40)
	expectSnapshot(40, 1)
	// Coalesced notifications: two publishes before the writer wakes yield
	// one fresh snapshot, never a stale intermediate one.
	stub.publish(60)
	stub.publish(80)
	expectSnapshot(80, 3)

	cancel()
	select {
	case <-stub.unsubscribe:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not unsubscribe after the client left")
	}
}

func TestTaskEventsFailsBeforeStreamingWhenSnapshotIsUnavailable(t *testing.T) {
	stub := &sseFailingTasks{}
	router := NewRouter(Dependencies{Tasks: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/tasks/events", nil))
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "内部服务错误") {
		t.Fatalf("response = %d %s", response.Code, response.Body)
	}
	if !stub.unsubscribed {
		t.Fatal("subscription leaked after the initial snapshot failed")
	}
}

type sseFailingTasks struct {
	unsubscribed bool
}

func (stub *sseFailingTasks) Subscribe() (<-chan struct{}, func()) {
	return make(chan struct{}), func() { stub.unsubscribed = true }
}

func (*sseFailingTasks) List(context.Context) ([]tasks.TaskInfo, error) {
	return nil, errors.New("database locked")
}

func (*sseFailingTasks) Revisions() tasks.TaskRevisions { return tasks.TaskRevisions{} }
