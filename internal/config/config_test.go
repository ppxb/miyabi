package config

import (
	"os"
	"strings"
	"testing"
)

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"MIYABI_LISTEN", "MIYABI_DATA_DIR", "MIYABI_LOG_LEVEL", "MIYABI_PROXY", "MIYABI_ACCESS_PASSWORD"} {
		// Restore the developer's environment when the test finishes.
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadDefaultsAndEnvironment(t *testing.T) {
	for _, test := range []struct {
		name string
		env  map[string]string
		want Config
	}{
		{name: "defaults", want: Config{Listen: ":8080", DataDir: "./data", LogLevel: "info"}},
		{
			name: "environment overrides defaults and preserves password whitespace",
			env: map[string]string{
				"MIYABI_LISTEN": " 127.0.0.1:9090 ", "MIYABI_DATA_DIR": " ./custom-data ",
				"MIYABI_LOG_LEVEL": " DEBUG ", "MIYABI_PROXY": "http://127.0.0.1:7890",
				"MIYABI_ACCESS_PASSWORD": " password with spaces ",
			},
			want: Config{Listen: "127.0.0.1:9090", DataDir: "./custom-data", LogLevel: "debug",
				Proxy: "http://127.0.0.1:7890", AccessPassword: " password with spaces "},
		},
		{
			name: "empty optional values disable proxy and access gate",
			env:  map[string]string{"MIYABI_PROXY": "", "MIYABI_ACCESS_PASSWORD": ""},
			want: Config{Listen: ":8080", DataDir: "./data", LogLevel: "info"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			clearConfigEnvironment(t)
			for key, value := range test.env {
				t.Setenv(key, value)
			}
			cfg, err := Load()
			if err != nil || cfg != test.want {
				t.Fatalf("config=%+v want=%+v err=%v", cfg, test.want, err)
			}
		})
	}
}

func TestLoadRejectsInvalidEnvironment(t *testing.T) {
	for _, test := range []struct{ key, value string }{
		{"MIYABI_LISTEN", ""}, {"MIYABI_LISTEN", "  "},
		{"MIYABI_DATA_DIR", ""}, {"MIYABI_DATA_DIR", "  "},
		{"MIYABI_LOG_LEVEL", ""}, {"MIYABI_LOG_LEVEL", "  "}, {"MIYABI_LOG_LEVEL", "trace"},
	} {
		t.Run(test.key+"="+test.value, func(t *testing.T) {
			clearConfigEnvironment(t)
			t.Setenv(test.key, test.value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), test.key) {
				t.Fatalf("expected an error naming %s, got %v", test.key, err)
			}
		})
	}
}

func TestLoadIgnoresLegacyConfigFile(t *testing.T) {
	clearConfigEnvironment(t)
	t.Chdir(t.TempDir())
	if err := os.WriteFile("config.toml", []byte("invalid legacy TOML ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if cfg, err := Load(); err != nil || cfg.Listen != ":8080" || cfg.DataDir != "./data" {
		t.Fatalf("local config file affected environment-only startup: %+v, %v", cfg, err)
	}
}
