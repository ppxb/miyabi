package download

import (
	"encoding/json"
	"testing"
	"time"
)

func TestConfigDefaultsPreserveExplicitOptOut(t *testing.T) {
	for _, tc := range []struct {
		input   string
		enabled bool
	}{{`{}`, true}, {`{"auto_switch":false}`, false}} {
		var cfg Config
		if err := json.Unmarshal([]byte(tc.input), &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.AutoSwitch != tc.enabled || cfg.Validate() != nil || cfg.MaxAttempts != 3 {
			t.Fatalf("unexpected defaults: %+v", cfg)
		}
		if cfg.Timeout(0) != 15*time.Minute || cfg.Timeout(21.0573) != 30*time.Minute || cfg.Timeout(99) != 60*time.Minute {
			t.Fatal("incorrect progress thresholds")
		}
	}
}
