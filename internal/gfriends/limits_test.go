package gfriends

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type spacesReader struct{}

func (spacesReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	return len(p), nil
}

type trackedBody struct {
	io.Reader
	read   int
	closed bool
}

func (b *trackedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}

func (b *trackedBody) Close() error {
	b.closed = true
	return nil
}

func TestDownloadSizeBoundaryAndMirrorFallback(t *testing.T) {
	const limit = 64
	paddedTree := testTree + strings.Repeat(" ", limit-len(testTree))
	for _, tc := range []struct {
		name          string
		contentLength int64
		oversized     bool
		wantRead      int
	}{
		{"exact limit", limit, false, limit},
		{"exact limit without length", -1, false, limit},
		{"declared oversize", limit + 1, true, 0},
		{"stream without length", -1, true, limit + 1},
		{"understated length", 1, true, limit + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(paddedTree)}
			if tc.oversized {
				// A complete JSON document followed by unbounded whitespace must
				// fail the size check even though a truncated prefix is valid JSON.
				body.Reader = io.MultiReader(strings.NewReader(testTree), spacesReader{})
			}
			calls, validations := 0, 0
			client := New("", &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != mirrors[calls-1]+"/Filetree.json" {
					t.Errorf("unexpected mirror: %s", r.URL)
				}
				if calls == 1 {
					return &http.Response{StatusCode: http.StatusOK, ContentLength: tc.contentLength, Body: body}, nil
				}
				return treeResponse(testTree), nil
			})})
			data, err := client.download(t.Context(), "Filetree.json", limit, func(data []byte) error {
				validations++
				_, err := parseIndex(data)
				return err
			})
			wantCalls, wantData := 1, paddedTree
			if tc.oversized {
				wantCalls, wantData = 2, testTree
			}
			if err != nil || string(data) != wantData || calls != wantCalls || validations != 1 {
				t.Fatalf("download = %q, %v; calls = %d, validations = %d", data, err, calls, validations)
			}
			if !body.closed || body.read != tc.wantRead {
				t.Fatalf("first response: closed = %t, read = %d, want %d", body.closed, body.read, tc.wantRead)
			}
		})
	}
}

func TestOversizedIndexStreamFallsBackAndCachesOnlyValidMirror(t *testing.T) {
	body := &trackedBody{Reader: io.MultiReader(strings.NewReader(testTree), spacesReader{})}
	calls := 0
	client := New(t.TempDir(), &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: http.StatusOK, ContentLength: -1, Body: body}, nil
		}
		return treeResponse(testTree), nil
	})})
	if err := client.EnsureIndex(t.Context()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !body.closed || body.read != maxIndexBytes+1 {
		t.Fatalf("calls = %d, closed = %t, bytes read = %d", calls, body.closed, body.read)
	}
	if _, ok := client.Lookup("Actor"); !ok {
		t.Fatal("valid mirror index missing")
	}
	data, err := os.ReadFile(filepath.Join(client.dataDir, "gfriends_tree.json"))
	if err != nil || string(data) != testTree {
		t.Fatalf("cached index = %q, %v", data, err)
	}
}

func TestOversizedAvatarFallsBackWithoutReturningTruncatedData(t *testing.T) {
	const limit = 10 << 20
	for _, length := range []int64{limit + 1, -1} {
		name, wantRead := "declared size", 0
		if length < 0 {
			name, wantRead = "stream without length", limit+1
		}
		t.Run(name, func(t *testing.T) {
			body := &trackedBody{Reader: spacesReader{}}
			calls := 0
			client := New("", &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return &http.Response{StatusCode: http.StatusOK, ContentLength: length, Body: body}, nil
				}
				return treeResponse("complete image"), nil
			})})
			client.installIndex(map[string]string{"actor": "Content/S/avatar.jpg"}, time.Now())
			data, err := client.FetchAvatar(t.Context(), "Actor")
			if err != nil || string(data) != "complete image" || calls != 2 {
				t.Fatalf("avatar: bytes = %d, error = %v, calls = %d", len(data), err, calls)
			}
			if !body.closed || body.read != wantRead {
				t.Fatalf("oversize avatar: closed = %t, read = %d, want %d", body.closed, body.read, wantRead)
			}
		})
	}
}

func TestOversizedIndexFailureBackoffAndRecovery(t *testing.T) {
	for _, cached := range []bool{false, true} {
		name := "first load"
		if cached {
			name = "stale cache"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "gfriends_tree.json")
			var stamp time.Time
			if cached {
				if err := os.WriteFile(path, []byte(testTree), 0644); err != nil {
					t.Fatal(err)
				}
				age := time.Now().Add(-2 * cacheExpiration)
				if err := os.Chtimes(path, age, age); err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				stamp = info.ModTime()
			}
			calls := 0
			client := New(dir, &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) {
				calls++
				if calls <= len(mirrors) {
					body := &trackedBody{Reader: spacesReader{}}
					t.Cleanup(func() {
						if !body.closed || body.read != 0 {
							t.Errorf("declared oversize: closed = %t, read = %d", body.closed, body.read)
						}
					})
					return &http.Response{StatusCode: http.StatusOK, ContentLength: maxIndexBytes + 1, Body: body}, nil
				}
				return treeResponse(testTree), nil
			})})
			for range 3 {
				err := client.EnsureIndex(t.Context())
				if cached && err != nil {
					t.Fatal(err)
				}
				if !cached && (err == nil || !strings.Contains(err.Error(), "exceeds")) {
					t.Fatalf("missing oversize error: %v", err)
				}
			}
			if calls != len(mirrors) || !client.loadedAt.Equal(stamp) || client.retryAt.IsZero() {
				t.Fatalf("calls = %d, loadedAt = %v, retryAt = %v", calls, client.loadedAt, client.retryAt)
			}
			if _, ok := client.Lookup("Actor"); ok != cached {
				t.Fatalf("cached actor present = %t, want %t", ok, cached)
			}
			data, err := os.ReadFile(path)
			if cached {
				if err != nil || string(data) != testTree {
					t.Fatalf("previous disk cache changed: %q, %v", data, err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("failed download created cache: %v", err)
			}
			client.retryAt = time.Now().Add(-time.Second)
			if err := client.EnsureIndex(t.Context()); err != nil {
				t.Fatal(err)
			}
			if calls != len(mirrors)+1 || !client.retryAt.IsZero() || client.refreshErr != nil {
				t.Fatal("retry failed to recover")
			}
			if _, ok := client.Lookup("Actor"); !ok {
				t.Fatal("recovered index missing")
			}
		})
	}
}

func TestCachedIndexSizeBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gfriends_tree.json")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	if _, err := io.WriteString(file, testTree); err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(file, spacesReader{}, maxIndexBytes-int64(len(testTree))); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	index, _, err := readCachedIndex(path)
	if err != nil || index["actor"] != "Content/S/avatar.jpg?t=1" {
		t.Fatalf("exact limit cache: %v", err)
	}
	if err := os.Truncate(path, maxIndexBytes+1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readCachedIndex(path); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversize cache error = %v", err)
	}
	calls := 0
	client := New(filepath.Dir(path), &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return treeResponse(testTree), nil
	})})
	if err := client.EnsureIndex(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, ok := client.Lookup("Actor"); !ok || calls != 1 {
		t.Fatalf("oversize cache suppressed download: lookup = %t, calls = %d", ok, calls)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != testTree {
		t.Fatalf("oversize cache not replaced: %d bytes, %v", len(data), err)
	}
}
