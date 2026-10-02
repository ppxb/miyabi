package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/config"
)

func TestBrowserCommandUsesThePlatformOpener(t *testing.T) {
	for goos, want := range map[string][2]string{
		"windows": {"rundll32", "url.dll,FileProtocolHandler"},
		"darwin":  {"open", ""},
		"linux":   {"xdg-open", ""},
	} {
		t.Run(goos, func(t *testing.T) {
			name, args := browserCommand(goos, "http://127.0.0.1:8080")
			if name != want[0] || args[len(args)-1] != "http://127.0.0.1:8080" {
				t.Fatalf("browserCommand(%q) = %q %v", goos, name, args)
			}
			if want[1] != "" && !reflect.DeepEqual(args, []string{want[1], "http://127.0.0.1:8080"}) {
				t.Fatalf("browserCommand(%q) = %q %v", goos, name, args)
			}
		})
	}
}

func TestPortableLoggerWritesTheDataDirectory(t *testing.T) {
	dataDir := t.TempDir()
	logger, closeLog := newLogger(config.Config{DataDir: dataDir, LogLevel: slog.LevelInfo}, true)
	logger.Info("portable startup", "listen", ":8080")
	closeLog()

	body, err := os.ReadFile(filepath.Join(dataDir, "miyabi.log"))
	if err != nil {
		t.Fatalf("log file was not written: %v", err)
	}
	if !strings.Contains(string(body), "portable startup") || !strings.Contains(string(body), ":8080") {
		t.Fatalf("log file = %s", body)
	}
}

func TestOpenLogFileRotatesOncePastTheLimit(t *testing.T) {
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "miyabi.log")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, logFileLimit+1); err != nil {
		t.Fatal(err)
	}

	file, err := openLogFile(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := io.WriteString(file, "rotated\n"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path + ".1"); err != nil || info.Size() != logFileLimit+1 {
		t.Fatalf("previous log was not rotated: %+v, %v", info, err)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "rotated\n" {
		t.Fatalf("fresh log = %q, %v", body, err)
	}
}
