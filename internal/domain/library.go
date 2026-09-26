package domain

// MovieSummary is the subset of catalogue metadata a subscription keeps.
type MovieSummary struct {
	ID          string
	Code        string
	Title       string
	Cover       string
	ReleaseDate string
}

// Media is a downloaded image payload with its content type.
type Media struct {
	ContentType string
	Body        []byte
}

// OfflineSubmission is the projected state of one 115 offline download workflow.
type OfflineSubmission struct {
	TaskID      int     `json:"task_id"`
	Code        string  `json:"code"`
	JavDBID     string  `json:"javdb_id"`
	LibraryID   int     `json:"library_id,omitempty"`
	AccountID   string  `json:"account_id"`
	DirectoryID string  `json:"directory_id"`
	ScanTaskID  int     `json:"scan_task_id,omitempty"`
	Hash        string  `json:"hash"`
	Status      string  `json:"status"`
	Phase       string  `json:"phase"`
	Processing  bool    `json:"processing"`
	Progress    int     `json:"progress"`
	Error       *string `json:"error,omitempty"`
}
