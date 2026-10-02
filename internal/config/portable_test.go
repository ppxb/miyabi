package config

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPortableStartup(t *testing.T) {
	for _, test := range []struct {
		name     string
		goos     string
		environ  []string
		existing bool
		want     bool
	}{
		{name: "double-clicked windows build", goos: "windows", want: true},
		{name: "explicit environment wins", goos: "windows", environ: []string{"MIYABI_LISTEN=:9000"}},
		{name: "unrelated variable does not opt out", goos: "windows", environ: []string{"PATH=/usr/bin"}, want: true},
		{name: "generated file pins the portable startup", goos: "windows", existing: true, want: true},
		{name: "other systems keep the documented defaults", goos: "linux"},
		{name: "container image variables win", goos: "linux", existing: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if test.existing {
				if err := os.WriteFile(filepath.Join(dir, portableFileName), []byte("MIYABI_LISTEN=:8080\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if got := Portable(test.goos, dir, test.environ); got != test.want {
				t.Fatalf("Portable(%q, %v) = %v, want %v", test.goos, test.environ, got, test.want)
			}
		})
	}
	if Portable("windows", "", nil) {
		t.Fatal("portable startup without an executable directory")
	}
}

func TestLoadPortableGeneratesAnEditableFile(t *testing.T) {
	clearConfigEnvironment(t)
	dir := t.TempDir()
	cfg, err := LoadPortable(dir)
	if err != nil {
		t.Fatal(err)
	}
	// The data directory follows the executable instead of the working directory.
	if cfg.DataDir != filepath.Join(dir, "data") || cfg.EmbyDir != filepath.Join(cfg.DataDir, "emby") {
		t.Fatalf("portable data defaults = %+v", cfg)
	}
	if cfg.Listen != ":8080" || cfg.LogLevel.String() != "INFO" || cfg.PublicURL == "" || !cfg.EmbySyncActors {
		t.Fatalf("portable defaults = %+v", cfg)
	}
	body, err := os.ReadFile(filepath.Join(dir, portableFileName))
	if err != nil {
		t.Fatalf("portable configuration was not generated: %v", err)
	}
	if !strings.Contains(string(body), cfg.DataDir) || !strings.Contains(string(body), "MIYABI_LISTEN=:8080") {
		t.Fatalf("generated configuration does not document the defaults:\n%s", body)
	}

	// A hand-edited file drives the next start.
	edited := "MIYABI_LISTEN=127.0.0.1:9090\nMIYABI_DATA_DIR=" + filepath.Join(dir, "media") + "\n"
	if err := os.WriteFile(filepath.Join(dir, portableFileName), []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadPortable(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:9090" || cfg.DataDir != filepath.Join(dir, "media") {
		t.Fatalf("edited configuration was ignored: %+v", cfg)
	}

	// An environment variable still wins over the file.
	t.Setenv("MIYABI_LISTEN", "127.0.0.1:7070")
	cfg, err = LoadPortable(dir)
	if err != nil || cfg.Listen != "127.0.0.1:7070" {
		t.Fatalf("environment did not override the file: %+v, %v", cfg, err)
	}
}

func TestLoadPortableReportsInvalidFileValues(t *testing.T) {
	clearConfigEnvironment(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, portableFileName), []byte("MIYABI_LOG_LEVEL=trace\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadPortable(dir)
	if err == nil || !strings.Contains(err.Error(), "MIYABI_LOG_LEVEL") {
		t.Fatalf("invalid file value was accepted: %v", err)
	}
}

func TestParseEnvFile(t *testing.T) {
	values := parseEnvFile([]byte(strings.Join([]string{
		"# comment",
		"",
		"   ",
		"MIYABI_LISTEN = :8080 ",
		`MIYABI_ACCESS_PASSWORD="pass word"`,
		"MIYABI_DATA_DIR=D:\\media\\miyabi",
		"MIYABI_STRM_TOKEN=a=b=c",
		"MIYABI_JWT_SECRET=",
		"MIYABI_EMBY_ENABLED",
		"\tMIYABI_EMBY_MEDIA_PATH= /media ",
	}, "\r\n")))
	want := map[string]string{
		"MIYABI_LISTEN":          ":8080",
		"MIYABI_ACCESS_PASSWORD": "pass word",
		"MIYABI_DATA_DIR":        `D:\media\miyabi`,
		"MIYABI_STRM_TOKEN":      "a=b=c",
		"MIYABI_EMBY_MEDIA_PATH": "/media",
	}
	if len(values) != len(want) {
		t.Fatalf("parsed %v, want %v", values, want)
	}
	for key, value := range want {
		if values[key] != value {
			t.Fatalf("%s = %q, want %q", key, values[key], value)
		}
	}
}

func TestExecutableDirIsUsable(t *testing.T) {
	dir, err := ExecutableDir()
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("ExecutableDir = %q, %v", dir, err)
	}
}

func TestParseEnvFileNormalizesEditorEncodings(t *testing.T) {
	const text = "MIYABI_LISTEN=127.0.0.1:9090\n"
	littleEndian, bigEndian := []byte{0xFF, 0xFE}, []byte{0xFE, 0xFF}
	for _, char := range text {
		littleEndian = binary.LittleEndian.AppendUint16(littleEndian, uint16(char))
		bigEndian = binary.BigEndian.AppendUint16(bigEndian, uint16(char))
	}
	for name, body := range map[string][]byte{
		"utf-8 with BOM": append([]byte{0xEF, 0xBB, 0xBF}, text...),
		"utf-16 le":      littleEndian,
		"utf-16 be":      bigEndian,
		"plain utf-8":    []byte(text),
	} {
		t.Run(name, func(t *testing.T) {
			values := parseEnvFile(body)
			if values["MIYABI_LISTEN"] != "127.0.0.1:9090" || len(values) != 1 {
				t.Fatalf("parsed %v", values)
			}
		})
	}
}
