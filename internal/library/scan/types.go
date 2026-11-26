package scan

import "github.com/ppxb/miyabi/internal/pan"

// Directory describes a directory queued during BFS traversal.
type Directory struct {
	ID   string
	Path string
}

// Video wraps a 115 file with an assigned catalogue code.
type Video struct {
	pan.File
	Code   string
	Manual bool
	// known marks a video whose unchanged database association already fixes
	// its movie identity, so sidecar NFO files do not need to be re-downloaded.
	known bool
	// DirectoryCode records a provisional part group for transactional validation.
	DirectoryCode string
}
