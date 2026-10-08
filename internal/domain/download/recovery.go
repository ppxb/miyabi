package download

import (
	"time"

	"github.com/ppxb/miyabi/internal/magnet"
)

// Attempt records distinct candidates within one user-visible download task.
type Attempt struct {
	Hash      string    `json:"hash"`
	InfoHash  string    `json:"info_hash,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

// Recovery is a durable checkpoint. A replacement is never submitted until
// removal of its predecessor has succeeded. The task ID remains unchanged.
type Recovery struct {
	Preferences       magnet.Preferences `json:"preferences"`
	Attempts          []Attempt          `json:"attempts"`
	CurrentHash       string             `json:"current_hash"`
	Action            string             `json:"action,omitempty"`
	NextHash          string             `json:"next_hash,omitempty"`
	RetryAt           time.Time          `json:"retry_at,omitempty"`
	ObservedAt        time.Time          `json:"observed_at,omitempty"`
	ProgressAt        time.Time          `json:"progress_at,omitempty"`
	Progress          float64            `json:"progress"`
	RemoteStatus      int                `json:"remote_status"`
	Stalled           bool               `json:"stalled,omitempty"`
	Exhausted         bool               `json:"exhausted,omitempty"`
	Reason            string             `json:"reason,omitempty"`
	Failures          int                `json:"failures,omitempty"`
	RemovalStarted    bool               `json:"removal_started,omitempty"`
	SubmissionStarted bool               `json:"submission_started,omitempty"`
	Manual            bool               `json:"manual,omitempty"`
}
