package export

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/nfo"
)

func TestRewriteSTRM(t *testing.T) {
	tempDir := t.TempDir()
	strmDir := filepath.Join(tempDir, "TEST-001")
	if err := os.MkdirAll(strmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	strmPath := filepath.Join(strmDir, "TEST-001.strm")
	oldContent := "http://127.0.0.1:8080/api/strm/play/12345?token=mytoken\n"
	if err := os.WriteFile(strmPath, []byte(oldContent), 0o644); err != nil {
		t.Fatal(err)
	}

	count, err := RewriteSTRM(t.Context(), tempDir, "http://10.32.217.101:8080", "mytoken")
	if err != nil {
		t.Fatalf("RewriteSTRM error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 file rewritten, got %d", count)
	}

	updated, err := os.ReadFile(strmPath)
	if err != nil {
		t.Fatal(err)
	}
	expected := "http://10.32.217.101:8080/api/strm/play/12345?token=mytoken\n"
	if string(updated) != expected {
		t.Fatalf("expected %q, got %q", expected, string(updated))
	}

	// Idempotent test: second run rewrites 0 files
	count2, err := RewriteSTRM(t.Context(), tempDir, "http://10.32.217.101:8080", "mytoken")
	if err != nil || count2 != 0 {
		t.Fatalf("expected 0 files rewritten on second run, got %d (err: %v)", count2, err)
	}

	// Token rotation test: rotating token to newtoken
	count3, err := RewriteSTRM(t.Context(), tempDir, "http://10.32.217.101:8080", "newtoken")
	if err != nil || count3 != 1 {
		t.Fatalf("expected 1 file rewritten on token rotation, got %d (err: %v)", count3, err)
	}
	updated3, err := os.ReadFile(strmPath)
	if err != nil {
		t.Fatal(err)
	}
	expectedRotated := "http://10.32.217.101:8080/api/strm/play/12345?token=newtoken\n"
	if string(updated3) != expectedRotated {
		t.Fatalf("expected %q, got %q", expectedRotated, string(updated3))
	}

	// Token clearing test: removing token
	count4, err := RewriteSTRM(t.Context(), tempDir, "http://10.32.217.101:8080", "")
	if err != nil || count4 != 1 {
		t.Fatalf("expected 1 file rewritten on token removal, got %d (err: %v)", count4, err)
	}
	updated4, err := os.ReadFile(strmPath)
	if err != nil {
		t.Fatal(err)
	}
	expectedNoToken := "http://10.32.217.101:8080/api/strm/play/12345\n"
	if string(updated4) != expectedNoToken {
		t.Fatalf("expected %q, got %q", expectedNoToken, string(updated4))
	}

	// Cancelled context test: stops early
	cancelledCtx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = RewriteSTRM(cancelledCtx, tempDir, "http://10.32.217.101:8080", "some-token")
	if err == nil {
		t.Fatal("expected context cancelled error, got nil")
	}
}

func TestParseSTRMFileID(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"http://127.0.0.1:8080/api/strm/play/12345", "12345"},
		{"http://127.0.0.1:8080/api/strm/play/12345?token=abc", "12345"},
		{"http://10.0.0.1:8080/api/strm/play/video-101\n", "video-101"},
		{"http://10.0.0.1:8080/api/strm/play/local-abcdef123456?token=secret\n", "local-abcdef123456"},
		{"invalid strm content", ""},
		{"http://127.0.0.1:8080/api/other/12345", ""},
	} {
		if got := ParseSTRMFileID(tc.input); got != tc.want {
			t.Errorf("ParseSTRMFileID(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestRewriteSTRMRejectsMissingDestinationsWithoutChangingFiles(t *testing.T) {
	for _, tc := range []struct {
		name, directory, publicURL string
	}{
		{"missing directory", "", "http://localhost:9090"},
		{"blank directory", " \t", "http://localhost:9090"},
		{"missing URL", "exports", ""},
		{"blank URL", "exports", " \t"},
		{"slash-only URL", "exports", " / "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			const original = "http://previous.example/api/strm/play/123?token=old\n"
			paths := []string{filepath.Join("data", "emby", "movie.strm"), filepath.Join("exports", "movie.strm")}
			for _, path := range paths {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(original), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if count, err := RewriteSTRM(t.Context(), tc.directory, tc.publicURL, "new"); err == nil || count != 0 {
				t.Fatalf("direct rewrite = %d, %v; want error without writes", count, err)
			}
			manager := NewManager(Config{EmbyDir: tc.directory, PublicURL: tc.publicURL, STRMToken: "new"})
			if count, err := manager.RewriteSTRM(t.Context()); err == nil || count != 0 {
				t.Fatalf("managed rewrite = %d, %v; want error without writes", count, err)
			}
			for _, path := range paths {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != original {
					t.Fatalf("existing STRM changed at %s: %q, %v", path, data, err)
				}
			}
		})
	}
}

func TestExportManager(t *testing.T) {
	mgr := NewManager(Config{
		EmbyDir:   "/tmp/emby",
		PublicURL: "http://example.com:8080",
		STRMToken: "secret",
	})
	cfg := mgr.Config()
	if cfg.EmbyDir != "/tmp/emby" || cfg.PublicURL != "http://example.com:8080" || cfg.STRMToken != "secret" {
		t.Fatalf("unexpected config: %+v", cfg)
	}

	mgr.Set(Config{
		EmbyDir:   "/tmp/emby2",
		PublicURL: "http://example2.com:8080",
		STRMToken: "secret2",
	})
	cfg2 := mgr.Config()
	if cfg2.EmbyDir != "/tmp/emby2" || cfg2.PublicURL != "http://example2.com:8080" || cfg2.STRMToken != "secret2" {
		t.Fatalf("unexpected updated config: %+v", cfg2)
	}
}

func TestEmbyMovieDir(t *testing.T) {
	tempDir := t.TempDir()

	for _, tc := range []struct {
		embyDir string
		code    string
		wantRel string
	}{
		{tempDir, "ALDN-613", filepath.Join(tempDir, "ALDN", "ALDN-613")},
		{tempDir, "A/B", filepath.Join(tempDir, "OTHERS", "A%2FB")},
		{tempDir, `A\B`, filepath.Join(tempDir, "OTHERS", "A%5CB")},
		{tempDir, "ABC:001", filepath.Join(tempDir, "OTHERS", "ABC%3A001")},
		{tempDir, "作品/限定 #007", filepath.Join(tempDir, "OTHERS", nfo.FileStem("作品/限定 #007"))},
	} {
		got := EmbyMovieDir(tc.embyDir, tc.code)
		if got != tc.wantRel {
			t.Errorf("EmbyMovieDir(%q, %q) = %q, want %q", tc.embyDir, tc.code, got, tc.wantRel)
		}

		// Verify the directory name is a single component under prefix directory (no sub-nesting)
		prefixDir := filepath.Dir(got)
		if filepath.Base(got) != nfo.FileStem(tc.code) {
			t.Errorf("expected leaf dir to be %q, got %q", nfo.FileStem(tc.code), filepath.Base(got))
		}
		if tc.embyDir != "" && filepath.Dir(prefixDir) != tc.embyDir {
			t.Errorf("expected parent to be prefix under %q, got %q", tc.embyDir, prefixDir)
		}

		// Ensure the directory can be created on the local file system without illegal path errors
		if tc.embyDir != "" {
			if err := os.MkdirAll(got, 0o755); err != nil {
				t.Errorf("os.MkdirAll(%q) failed for code %q: %v", got, tc.code, err)
			}
		}
	}
}

func TestEmbyMovieDirStaysWithinRoot(t *testing.T) {
	root := t.TempDir()
	for _, code := range []string{"../../ABC-001", `..\..\ABC-001`, "A/B-001", ".", "..", "..-001"} {
		got := EmbyMovieDir(root, code)
		rel, err := filepath.Rel(root, got)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.Dir(filepath.Dir(got)) != root {
			t.Errorf("unsafe export path for %q: %s (%v)", code, got, err)
		}
	}
}

func TestManagerSerializesExportsAndRewrites(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(Config{EmbyDir: root, PublicURL: "http://old.example", STRMToken: "old"})
	path := filepath.Join(root, "movie.strm")
	started, release := make(chan struct{}), make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		err := mgr.WithConfig(func(cfg Config) error {
			close(started)
			<-release
			return os.WriteFile(path, STRMContent(cfg.PublicURL, "101", cfg.STRMToken), 0644)
		})
		if err != nil {
			t.Error(err)
		}
	})
	<-started
	updating, updated := make(chan struct{}), make(chan struct{})
	wg.Go(func() {
		close(updating)
		mgr.Set(Config{EmbyDir: root, PublicURL: "http://new.example", STRMToken: "new"})
		close(updated)
		if _, err := mgr.RewriteSTRM(context.Background()); err != nil {
			t.Error(err)
		}
	})
	<-updating
	select {
	case <-updated:
		t.Error("configuration changed during export")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(STRMContent("http://new.example", "101", "new")) {
		t.Fatalf("stale export survived rewrite: %s (%v)", data, err)
	}
}
