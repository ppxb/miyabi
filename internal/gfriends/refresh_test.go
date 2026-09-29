package gfriends

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func treeResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
}

const testTree = `{"Content":{"S":{"Actor.jpg":"avatar.jpg?t=1"}}}`

func TestRefreshLeavesIndexReadableAndCoalescesCallers(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	defer unblock.Do(func() { close(release) })
	var calls atomic.Int32
	c := New("", &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return treeResponse(testTree), nil
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	})})
	c.installIndex(FileTree{Content: map[string]map[string]string{"S": {"Old.jpg": "old.jpg"}}}, time.Now().Add(-2*CacheExpiration))
	done := make(chan error, 9)
	go func() { done <- c.EnsureIndex(t.Context()) }()
	<-started
	lookup := make(chan bool, 1)
	go func() { _, ok := c.Lookup("Old"); lookup <- ok }()
	select {
	case ok := <-lookup:
		if !ok {
			t.Fatal("old index disappeared during refresh")
		}
	case <-time.After(time.Second):
		t.Fatal("lookup blocked behind download")
	}
	canceled, cancel := context.WithCancel(t.Context())
	waiting := make(chan error, 1)
	go func() { waiting <- c.EnsureIndex(canceled) }()
	cancel()
	select {
	case err := <-waiting:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waiter = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled waiter blocked")
	}
	for range 8 {
		go func() { done <- c.EnsureIndex(t.Context()) }()
	}
	unblock.Do(func() { close(release) })
	for range 9 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("downloads = %d", calls.Load())
	}
	if _, ok := c.Lookup("Actor"); !ok {
		t.Fatal("new index missing")
	}
	if _, ok := c.Lookup("Old"); ok {
		t.Fatal("old index retained after replacement")
	}
}

func TestDiskCacheKeepsItsAgeAndRetriesAfterFailure(t *testing.T) {
	for _, age := range []time.Duration{time.Hour, 2 * CacheExpiration} {
		t.Run(age.String(), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "gfriends_tree.json")
			if err := os.WriteFile(path, []byte(testTree), 0644); err != nil {
				t.Fatal(err)
			}
			stamp := time.Now().Add(-age)
			if err := os.Chtimes(path, stamp, stamp); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			c := New(dir, &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return nil, errors.New("offline")
			})})
			for range 3 {
				if err := c.EnsureIndex(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			if !c.loadedAt.Equal(info.ModTime()) {
				t.Fatalf("cache age reset: %v", c.loadedAt)
			}
			if _, ok := c.Lookup("Actor"); !ok {
				t.Fatal("disk fallback missing")
			}
			if age < CacheExpiration {
				if calls.Load() != 0 {
					t.Fatal("fresh disk cache downloaded again")
				}
				return
			}
			if calls.Load() != int32(len(mirrors)) {
				t.Fatalf("backoff did not suppress downloads: %d", calls.Load())
			}
			c.mu.Lock()
			c.retryAt = time.Now().Add(-time.Second)
			c.mu.Unlock()
			if err := c.EnsureIndex(t.Context()); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != int32(2*len(mirrors)) {
				t.Fatalf("expired retry did not download: %d", calls.Load())
			}
		})
	}
}

func TestMirrorFallbackValidatesIndexAndPreservesAvatarURL(t *testing.T) {
	var indexCalls, imageCalls int
	c := New("", &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/Filetree.json") {
			indexCalls++
			if indexCalls == 1 {
				return treeResponse("invalid JSON"), nil
			}
			return treeResponse(testTree), nil
		}
		imageCalls++
		if !strings.HasSuffix(r.URL.Path, "/Content/S/avatar.jpg") || r.URL.RawQuery != "t=1" {
			t.Errorf("avatar URL = %s", r.URL)
		}
		if imageCalls == 1 {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
		}
		return treeResponse("image"), nil
	})})
	data, err := c.FetchAvatar(t.Context(), "Actor")
	if err != nil || string(data) != "image" {
		t.Fatalf("avatar = %q, %v", data, err)
	}
	if indexCalls != 2 || imageCalls != 2 {
		t.Fatalf("mirror attempts = %d/%d", indexCalls, imageCalls)
	}
}

func TestMissingIndexFailureBackoffAndRecovery(t *testing.T) {
	var calls int
	c := New("", &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) {
		calls++
		if calls <= len(mirrors) {
			return nil, errors.New("offline")
		}
		return treeResponse(testTree), nil
	})})
	for range 3 {
		if err := c.EnsureIndex(t.Context()); err == nil {
			t.Fatal("missing index failure hidden")
		}
	}
	if calls != len(mirrors) {
		t.Fatalf("failure retried without backoff: %d", calls)
	}
	c.retryAt = time.Now().Add(-time.Second)
	if err := c.EnsureIndex(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Lookup("Actor"); !ok {
		t.Fatal("recovery did not install index")
	}
	if c.refreshErr != nil || !c.retryAt.IsZero() {
		t.Fatal("recovery retained failure state")
	}
}

func TestCanceledRefreshDoesNotDelayNextAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var calls int
	c := New("", &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			cancel()
			return nil, r.Context().Err()
		}
		return treeResponse(testTree), nil
	})})
	if err := c.EnsureIndex(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("refresh = %v", err)
	}
	if !c.retryAt.IsZero() {
		t.Fatal("caller cancellation set failure backoff")
	}
	if err := c.EnsureIndex(t.Context()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("requests = %d", calls)
	}
}
