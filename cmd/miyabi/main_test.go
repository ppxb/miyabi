package main

import (
	"strings"
	"testing"
)

func TestRunRejectsUnsupportedArgumentsBeforeLoadingConfig(t *testing.T) {
	// Even if argument validation regresses, do not start any application services.
	t.Setenv("MIYABI_LOG_LEVEL", "invalid-test-level")
	for _, args := range [][]string{
		{"-config", "config.toml"}, {"-listen", ":9090"}, {"-data-dir", "./other"},
		{"-log-level", "debug"}, {"-proxy", "http://127.0.0.1:7890"},
		{"unknown"}, {"healthcheck", "-listen", ":9090"}, {"healthcheck", "extra"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if err := run(args); err == nil || !strings.Contains(err.Error(), "usage: miyabi [healthcheck]") ||
				!strings.Contains(err.Error(), "MIYABI_*") {
				t.Fatalf("unsupported arguments did not return configuration guidance: %v", err)
			}
		})
	}
}
