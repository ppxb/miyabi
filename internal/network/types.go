package network

// ProbeResult reports the test status and latency of an upstream target.
type ProbeResult struct {
	Available bool   `json:"available"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// TestResult contains probe results for the JavBus target.
type TestResult struct {
	JavBus ProbeResult `json:"javbus"`
}
