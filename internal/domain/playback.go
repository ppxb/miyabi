package domain

// LibraryFile is an indexed video file inside a mounted media source.
type LibraryFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}
