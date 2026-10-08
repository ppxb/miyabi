package download

import (
	"encoding/json"
	"testing"
)

func TestConfigDefaultsPreserveExplicitOptOut(t *testing.T) {
	for _, tc := range []struct {
		input   string
		enabled bool
	}{{`{}`, true}, {`{"auto_switch":false}`, false}, {`{"auto_switch":false,"zero_progress_minutes":1,"max_attempts":10}`, false}} {
		var cfg Config
		if err := json.Unmarshal([]byte(tc.input), &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.AutoSwitch != tc.enabled {
			t.Fatalf("unexpected defaults: %+v", cfg)
		}
		encoded, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		if len(fields) != 1 || fields["auto_switch"] != tc.enabled {
			t.Fatalf("unexpected public settings: %s", encoded)
		}
	}
}
