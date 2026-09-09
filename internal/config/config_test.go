package config

import (
	"os"
	"path/filepath"
	"testing"
)

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
