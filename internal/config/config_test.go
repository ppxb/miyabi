package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/netx"
)

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"MIYABI_LISTEN", "MIYABI_DATA_DIR", "MIYABI_EMBY_DIR",
		"MIYABI_PUBLIC_URL", "MIYABI_STRM_TOKEN",
		"MIYABI_LOG_LEVEL", "MIYABI_ACCESS_PASSWORD", "MIYABI_JWT_SECRET", "MIYABI_TRUSTED_PROXIES",
		"MIYABI_EMBY_ENABLED", "MIYABI_EMBY_SERVER_URL", "MIYABI_EMBY_API_KEY", "MIYABI_EMBY_MEDIA_PATH", "MIYABI_EMBY_SYNC_ACTORS",
	} {
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
		{
			name: "defaults",
			want: Config{
				Listen:    ":8080",
				DataDir:   "./data",
				EmbyDir:   filepath.Join("./data", "emby"),
				PublicURL: "http://" + netx.OutboundIP() + ":8080",
				LogLevel:  "info",
				Runtime:        DefaultRuntime(),
				EmbySyncActors: true,
			},
		},
		{
			name: "environment overrides defaults and preserves password whitespace",
			env: map[string]string{
				"MIYABI_LISTEN":         " 127.0.0.1:9090 ",
				"MIYABI_DATA_DIR":       " ./custom-data ",
				"MIYABI_EMBY_DIR":       " ./custom-emby ",
				"MIYABI_PUBLIC_URL":     " http://192.168.1.100:8080/ ",
				"MIYABI_STRM_TOKEN":     " secret-token ",
				"MIYABI_LOG_LEVEL":      " DEBUG ",
				"MIYABI_ACCESS_PASSWORD": " password with spaces ",
				"MIYABI_EMBY_ENABLED":   " true ",
				"MIYABI_EMBY_SERVER_URL": " http://192.168.1.50:8096/ ",
				"MIYABI_EMBY_API_KEY":    " my-emby-key ",
				"MIYABI_EMBY_MEDIA_PATH": " /media ",
				"MIYABI_EMBY_SYNC_ACTORS": " false ",
			},
			want: Config{
				Listen:         "127.0.0.1:9090",
				DataDir:        "./custom-data",
				EmbyDir:        "./custom-emby",
				PublicURL:      "http://192.168.1.100:8080",
				STRMToken:      "secret-token",
				LogLevel:       "debug",
				AccessPassword: " password with spaces ",
				Runtime:        DefaultRuntime(),
				EmbyEnabled:    true,
				EmbyServerURL:  "http://192.168.1.50:8096",
				EmbyAPIKey:     "my-emby-key",
				EmbyMediaPath:  "/media",
				EmbySyncActors: false,
			},
		},
		{
			name: "empty optional value disables access gate",
			env:  map[string]string{"MIYABI_ACCESS_PASSWORD": ""},
			want: Config{
				Listen:         ":8080",
				DataDir:        "./data",
				EmbyDir:        filepath.Join("./data", "emby"),
				PublicURL:      "http://" + netx.OutboundIP() + ":8080",
				LogLevel:       "info",
				Runtime:        DefaultRuntime(),
				EmbySyncActors: true,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			clearConfigEnvironment(t)
			for key, value := range test.env {
				t.Setenv(key, value)
			}
			cfg, err := Load()
			if err != nil || !reflect.DeepEqual(cfg, test.want) {
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
