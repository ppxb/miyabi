package domain

// ScanPayload describes the stored JSON payload of a library scan task.
type ScanPayload struct {
	MovieID       int           `json:"movie_id,omitempty"`
	Rebuild       bool          `json:"rebuild,omitempty"`
	ScanID        string        `json:"scan_id,omitempty"`
	Source        LibrarySource `json:"source"`
	Scan          ScanProgress  `json:"scan"`
	TargetID      string        `json:"target_id,omitempty"`
	TargetPath    string        `json:"target_path,omitempty"`
	TargetFile    bool          `json:"target_file,omitempty"`
	OfflineTaskID int           `json:"offline_task_id,omitempty"`
	Code          string        `json:"code,omitempty"`
	JavDBID       string        `json:"javdb_id,omitempty"`
	Checkpoint    string        `json:"checkpoint,omitempty"`
	// ReusedTasks links this scan to work already owned by another scan.
	ReusedTasks []int `json:"reused_tasks,omitempty"`
}
