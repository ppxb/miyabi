package export

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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

	mgr.Update(func(old Config) Config {
		old.PublicURL = "http://updated.com:8080"
		return old
	})
	cfg3 := mgr.Config()
	if cfg3.PublicURL != "http://updated.com:8080" || cfg3.STRMToken != "secret2" {
		t.Fatalf("unexpected atomic update: %+v", cfg3)
	}
}
