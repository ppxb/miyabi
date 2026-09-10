package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMinimumVideoSizeConfiguration(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		value string
		want  int64
		valid bool
	}{
		{"", 100, true},
		{"0", 0, true},
		{"250", 250, true},
		{"-1", 0, false},
		{"8796093022208", 0, false},
	} {
		t.Run(scenario.value, func(t *testing.T) {
			t.Setenv("MIYABI_MIN_VIDEO_SIZE_MB", scenario.value)
			if scenario.value == "" {
				if err := os.Unsetenv("MIYABI_MIN_VIDEO_SIZE_MB"); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := Load([]string{"-config", configPath})
			if (err == nil) != scenario.valid || err == nil && cfg.MinVideoSizeMB != scenario.want {
				t.Fatalf("minimum size=%d want=%d err=%v", cfg.MinVideoSizeMB, scenario.want, err)
			}
		})
	}
}

func TestAccessPasswordConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte("access_password = ' file-password '\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		env      string
		unset    bool
		expected string
	}{
		{name: "file", unset: true, expected: " file-password "},
		{name: "environment overrides file", env: " env-password ", expected: " env-password "},
		{name: "empty environment disables gate"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Setenv restores any real startup value after the test.
			t.Setenv("MIYABI_ACCESS_PASSWORD", test.env)
			if test.unset {
				if err := os.Unsetenv("MIYABI_ACCESS_PASSWORD"); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := Load([]string{"-config", configPath})
			if err != nil {
				t.Fatal(err)
			}
			if cfg.AccessPassword != test.expected {
				t.Fatalf("unexpected access password: %q", cfg.AccessPassword)
			}
		})
	}
}
