package config

import (
	"testing"
	"time"
)

func TestDefaultRuntime(t *testing.T) {
	rt := DefaultRuntime()
	if rt.TaskPoolWorkers != 1 {
		t.Fatalf("task pool must stay single-worker to keep scans ordered, got %d", rt.TaskPoolWorkers)
	}
	for name, value := range map[string]time.Duration{
		"OfflineSyncInterval":  rt.OfflineSyncInterval,
		"MonitorCheckInterval": rt.MonitorCheckInterval,
		"OfflineSubmitTimeout": rt.OfflineSubmitTimeout,
	} {
		if value <= 0 {
			t.Fatalf("%s must be positive, got %v", name, value)
		}
	}
}
