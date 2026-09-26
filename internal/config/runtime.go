package config

import "time"

// Runtime holds the operational constants the composition root wires into
// services. Only values that app.New actually passes on live here; upstream
// clients (pan, javdb) and caches keep their own named constants.
type Runtime struct {
	// TaskPoolWorkers is the number of concurrent task handlers. Scans,
	// metadata writes and sidecar uploads must stay ordered, so it is 1.
	TaskPoolWorkers int
	// OfflineSyncInterval is how often 115 offline tasks are polled.
	OfflineSyncInterval time.Duration
	// MonitorCheckInterval is how often subscriptions are checked for magnets.
	MonitorCheckInterval time.Duration
	// OfflineSubmitTimeout bounds one 115 offline submission after the
	// request context has been detached from the caller.
	OfflineSubmitTimeout time.Duration
}

// DefaultRuntime returns the production values.
func DefaultRuntime() Runtime {
	return Runtime{
		TaskPoolWorkers:      1,
		OfflineSyncInterval:  30 * time.Second,
		MonitorCheckInterval: 5 * time.Minute,
		OfflineSubmitTimeout: 2 * time.Minute,
	}
}
