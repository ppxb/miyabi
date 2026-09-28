package domain

import "time"

// TaskInfo is the API presentation of a scan workflow and folded background tasks.
type TaskInfo struct {
	ID            int           `json:"id"`
	Type          string        `json:"type"`
	Status        string        `json:"status"`
	Progress      int           `json:"progress"`
	Error         *string       `json:"error,omitempty"`
	CanRetry      bool          `json:"can_retry,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	Source        LibrarySource `json:"source"`
	Scan          ScanProgress  `json:"scan"`
	OfflineTaskID int           `json:"offline_task_id,omitempty"`
	// Batch is set for subscription batch tasks, which have no scan payload.
	Batch *SubscriptionBatch `json:"batch,omitempty"`
}
