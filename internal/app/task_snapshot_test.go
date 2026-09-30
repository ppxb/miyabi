package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/api"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/monitor"
)

func snapshotFixture(t *testing.T) (*taskViews, testLibraryFixture) {
	t.Helper()
	fix := libraryFixture(t)
	return &taskViews{Service: fix.Tasks, database: fix.DB, library: fix.Service, monitor: monitor.New(fix.DB, nil, nil, fix.Tasks)}, fix
}

type snapshotResult struct {
	items []domain.TaskInfo
	err   error
}

func readSnapshot(ctx context.Context, views *taskViews) <-chan snapshotResult {
	result := make(chan snapshotResult, 1)
	go func() {
		items, err := views.List(ctx)
		result <- snapshotResult{items, err}
	}()
	return result
}

func TestTaskSnapshotsShareQueriesAndRefreshOnEveryNotification(t *testing.T) {
	views, fix := snapshotFixture(t)
	var queries atomic.Int32
	fix.DB.Task.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			queries.Add(1)
			return next.Query(ctx, query)
		})
	}))
	want, err := views.list(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	perLoad := queries.Swap(0)
	if perLoad == 0 {
		t.Fatal("fixture did not query tasks")
	}
	var readers []<-chan snapshotResult
	for range 20 {
		readers = append(readers, readSnapshot(t.Context(), views))
	}
	for _, reader := range readers {
		got := <-reader
		if got.err != nil || !reflect.DeepEqual(got.items, want) {
			t.Fatalf("snapshot=%+v err=%v", got.items, got.err)
		}
	}
	if queries.Load() != perLoad {
		t.Fatalf("20 readers made %d queries, want one load (%d)", queries.Load(), perLoad)
	}
	for i, notify := range []func(){fix.Tasks.Notify, fix.Tasks.NotifyLibraryChanged, fix.Tasks.NotifyOfflineChanged, fix.Tasks.NotifyMonitorChanged} {
		fix.DB.Task.UpdateOneID(fix.Queued.ID).SetProgress(i + 1).ExecX(t.Context())
		notify()
		queries.Store(0)
		for range 2 {
			items, err := views.List(t.Context())
			if err != nil || items[0].Progress != i+1 {
				t.Fatalf("stale progress: %+v, %v", items, err)
			}
		}
		if queries.Load() != perLoad {
			t.Fatalf("notification made %d queries, want %d", queries.Load(), perLoad)
		}
	}
}

func TestTaskSnapshotReadDoesNotConsumeANewerNotification(t *testing.T) {
	views, fix := snapshotFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	var first atomic.Bool
	fix.DB.Task.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			value, err := next.Query(ctx, query)
			if first.CompareAndSwap(false, true) {
				close(started)
				<-release
			}
			return value, err
		})
	}))
	pending := readSnapshot(t.Context(), views)
	<-started
	fix.DB.Task.UpdateOneID(fix.Queued.ID).SetProgress(55).ExecX(t.Context())
	fix.Tasks.Notify()
	release <- struct{}{}
	if got := <-pending; got.err != nil || got.items[0].Progress != 0 {
		t.Fatalf("first read=%+v err=%v", got.items, got.err)
	}
	items, err := views.List(t.Context())
	if err != nil || items[0].Progress != 55 {
		t.Fatalf("update during read was lost: %+v, %v", items, err)
	}
}

func TestCanceledTaskSnapshotReaderDoesNotFailOtherReaders(t *testing.T) {
	views, fix := snapshotFixture(t)
	ctx, stop := context.WithTimeout(t.Context(), 5*time.Second)
	defer stop()
	firstCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	started := make(chan struct{})
	var first atomic.Bool
	fix.DB.Task.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			if first.CompareAndSwap(false, true) {
				close(started)
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return next.Query(ctx, query)
		})
	}))
	loading := readSnapshot(firstCtx, views)
	<-started
	healthy := readSnapshot(ctx, views)
	waiterCtx, cancelWaiter := context.WithCancel(ctx)
	waiter := readSnapshot(waiterCtx, views)
	cancelWaiter()
	if got := <-waiter; !errors.Is(got.err, context.Canceled) {
		t.Fatalf("canceled waiter: %v", got.err)
	}
	cancel()
	if got := <-loading; !errors.Is(got.err, context.Canceled) {
		t.Fatalf("canceled loader: %v", got.err)
	}
	if got := <-healthy; got.err != nil || len(got.items) != 1 {
		t.Fatalf("healthy reader: %+v, %v", got.items, got.err)
	}
}

func TestFailedTaskSnapshotIsNotCached(t *testing.T) {
	views, fix := snapshotFixture(t)
	failure := errors.New("fixture query failure")
	var first atomic.Bool
	fix.DB.Task.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			if first.CompareAndSwap(false, true) {
				return nil, failure
			}
			return next.Query(ctx, query)
		})
	}))
	if _, err := views.List(t.Context()); !errors.Is(err, failure) {
		t.Fatalf("query failure=%v", err)
	}
	if items, err := views.List(t.Context()); err != nil || len(items) != 1 {
		t.Fatalf("query retry=%+v, %v", items, err)
	}
}

func TestTaskStreamsShareSnapshotsAndReconnectWithCurrentProgress(t *testing.T) {
	views, fix := snapshotFixture(t)
	var queries atomic.Int32
	fix.DB.Task.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			queries.Add(1)
			return next.Query(ctx, query)
		})
	}))
	server := httptest.NewServer(api.NewRouter(api.Dependencies{
		Access: api.NewAccessGateService("", ""), Tasks: views,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	connect := func() (*http.Response, *bufio.Reader) {
		t.Helper()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/tasks/events", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			t.Fatalf("stream status=%d", response.StatusCode)
		}
		return response, bufio.NewReader(response.Body)
	}
	expectProgress := func(reader *bufio.Reader, progress int) {
		t.Helper()
		isTasks := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(line, "event:") {
				isTasks = strings.TrimSpace(strings.TrimPrefix(line, "event:")) == "tasks"
			}
			if isTasks && strings.HasPrefix(line, "data:") {
				var items []domain.TaskInfo
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data:")), &items); err != nil || len(items) != 1 || items[0].Progress != progress {
					t.Fatalf("stream progress=%s want=%d err=%v", line, progress, err)
				}
				return
			}
		}
	}
	first, firstReader := connect()
	defer first.Body.Close()
	expectProgress(firstReader, 0)
	perLoad := queries.Load()
	second, secondReader := connect()
	defer second.Body.Close()
	expectProgress(secondReader, 0)
	if perLoad == 0 || queries.Load() != perLoad {
		t.Fatal("initial streams did not share one query round")
	}
	fix.DB.Task.UpdateOneID(fix.Queued.ID).SetProgress(42).ExecX(ctx)
	fix.Tasks.Notify()
	expectProgress(firstReader, 42)
	expectProgress(secondReader, 42)
	if queries.Load() != 2*perLoad {
		t.Fatal("a shared notification caused duplicate queries")
	}
	first.Body.Close()
	reconnected, reader := connect()
	defer reconnected.Body.Close()
	expectProgress(reader, 42)
	if queries.Load() != 2*perLoad {
		t.Fatal("reconnection did not reuse the current snapshot")
	}
	fix.DB.Task.UpdateOneID(fix.Queued.ID).SetProgress(70).ExecX(ctx)
	fix.Tasks.Notify()
	expectProgress(secondReader, 70)
	expectProgress(reader, 70)
	if queries.Load() != 3*perLoad {
		t.Fatal("disconnect disrupted snapshot sharing")
	}
}
