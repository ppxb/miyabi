package domain

// LocalAccountID identifies files and scan tasks in the local library.
const LocalAccountID = "local"

// LibraryDirectory describes a mounted media directory on remote storage.
type LibraryDirectory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// LibrarySource uniquely identifies a mounted media storage location and account.
type LibrarySource struct {
	AccountID string           `json:"account_id"`
	Directory LibraryDirectory `json:"directory"`
}

// ScanProgress records the state and statistics of a library scan.
type ScanProgress struct {
	Stage                 string `json:"stage"`
	CurrentPath           string `json:"current_path"`
	DirectoriesDiscovered int    `json:"directories_discovered"`
	DirectoriesScanned    int    `json:"directories_scanned"`
	FilesScanned          int    `json:"files_scanned"`
	VideoFiles            int    `json:"video_files"`
	MatchedFiles          int    `json:"matched_files"`
	UnmatchedFiles        int    `json:"unmatched_files"`
	Movies                int    `json:"movies"`
	RemovedFiles          int    `json:"removed_files"`
	RemovedMovies         int    `json:"removed_movies"`
	MetadataTotal         int    `json:"metadata_total"`
	MetadataCompleted     int    `json:"metadata_completed"`
	MetadataRetrying      int    `json:"metadata_retrying,omitempty"`
	MetadataFailed        int    `json:"metadata_failed,omitempty"`
}
